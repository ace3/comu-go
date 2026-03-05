(function (root) {
  const DEFAULTS = {
    minTransferMs: 2 * 60 * 1000,
    maxTransferMs: 120 * 60 * 1000,
    maxTransfers: 3,
    maxCandidateDepartures: 60,
    maxForwardStops: 30,
    maxQueueSize: 1200,
    maxExpandedStates: 1500,
    maxRuntimeMs: 7000,
    maxResults: 8,
    lookbackMs: 10 * 60 * 1000,
    lookaheadMs: 6 * 60 * 60 * 1000,
  };

  function toTime(value) {
    return value instanceof Date ? value : new Date(value);
  }

  function diffMinutes(from, to) {
    return Math.max(0, Math.round((to.getTime() - from.getTime()) / 60000));
  }

  function getStopTime(stop) {
    const primary = stop.arrives_at || stop.departs_at;
    return toTime(primary);
  }

  function findStopIndex(route, stationID, minTime) {
    for (let i = 0; i < route.length; i++) {
      const stop = route[i];
      if (String(stop.station_id || "").toUpperCase() !== stationID) {
        continue;
      }
      if (!minTime || toTime(stop.departs_at || stop.arrives_at) >= minTime) {
        return i;
      }
    }
    return route.findIndex((stop) => String(stop.station_id || "").toUpperCase() === stationID);
  }

  function optionKey(option) {
    return option.legs.map((leg) => `${leg.trainId}:${leg.from}->${leg.to}@${leg.departAt.toISOString()}`).join("|");
  }

  function sortOptions(a, b) {
    const transferDiff = (a.legs.length - b.legs.length);
    if (transferDiff !== 0) {
      return transferDiff;
    }
    const arriveDiff = a.arriveAt - b.arriveAt;
    if (arriveDiff !== 0) {
      return arriveDiff;
    }
    const departDiff = a.departAt - b.departAt;
    if (departDiff !== 0) {
      return departDiff;
    }
    return a.durationMinutes - b.durationMinutes;
  }

  function finalizeOptions(options, maxResults) {
    if (!Array.isArray(options) || options.length === 0) {
      return [];
    }
    const sorted = [...options].sort(sortOptions);
    const direct = sorted.filter((option) => option.legs.length === 1);
    if (direct.length > 0) {
      return direct.slice(0, maxResults);
    }
    const minLegs = sorted[0].legs.length;
    return sorted.filter((option) => option.legs.length === minLegs).slice(0, maxResults);
  }

  async function findTripOptions(input) {
    const config = { ...DEFAULTS, ...(input?.config || {}) };
    const fromID = String(input?.fromID || "").toUpperCase();
    const toID = String(input?.toID || "").toUpperCase();
    const now = toTime(input?.now || new Date());

    if (!fromID || !toID || fromID === toID) {
      return { options: [], stats: { expandedStates: 0, truncated: false } };
    }

    const getRoute = input?.getRoute;
    const getStationSchedules = input?.getStationSchedules;
    if (typeof getRoute !== "function" || typeof getStationSchedules !== "function") {
      throw new Error("findTripOptions requires getRoute and getStationSchedules functions");
    }

    const firstLegSchedules = Array.isArray(input?.firstLegSchedules) ? input.firstLegSchedules : [];
    const minDeparture = new Date(now.getTime() - config.lookbackMs);
    const maxDeparture = new Date(now.getTime() + config.lookaheadMs);

    const firstLegCandidates = firstLegSchedules
      .filter((schedule) => {
        const departAt = toTime(schedule.departs_at);
        return departAt >= minDeparture && departAt <= maxDeparture;
      })
      .sort((a, b) => toTime(a.departs_at) - toTime(b.departs_at))
      .slice(0, config.maxCandidateDepartures);

    const queue = [];
    const options = [];
    const seen = new Set();
    let expandedStates = 0;
    const startedAt = Date.now();
    let truncated = false;

    for (const first of firstLegCandidates) {
      const departAt = toTime(first.departs_at);
      const route = await getRoute(first.train_id);
      if (!Array.isArray(route) || route.length === 0) {
        continue;
      }
      const fromIdx = findStopIndex(route, fromID, new Date(departAt.getTime() - config.minTransferMs));
      if (fromIdx < 0) {
        continue;
      }
      const maxForwardStops = Math.min(route.length, fromIdx + config.maxForwardStops);
      for (let i = fromIdx + 1; i < maxForwardStops; i++) {
        const nextStation = String(route[i].station_id || "").toUpperCase();
        if (!nextStation || nextStation === fromID) {
          continue;
        }
        const arriveAt = getStopTime(route[i]);
        queue.push({
          stationID: nextStation,
          arriveAt,
          legs: [{
            trainId: first.train_id,
            line: first.line,
            from: fromID,
            to: nextStation,
            departAt,
            arriveAt,
          }],
          visited: new Set([fromID, nextStation]),
        });
        if (queue.length >= config.maxQueueSize) {
          truncated = true;
          break;
        }
      }
      if (queue.length >= config.maxQueueSize) {
        truncated = true;
        break;
      }
    }

    while (queue.length > 0 && options.length < config.maxResults * 5) {
      if (Date.now() - startedAt > config.maxRuntimeMs || expandedStates >= config.maxExpandedStates) {
        truncated = true;
        break;
      }
      queue.sort((a, b) => a.arriveAt - b.arriveAt);
      const current = queue.shift();
      expandedStates += 1;

      const legs = current.legs;
      const lastLeg = legs[legs.length - 1];
      if (current.stationID === toID) {
        const option = {
          legs,
          departAt: legs[0].departAt,
          arriveAt: lastLeg.arriveAt,
          durationMinutes: diffMinutes(legs[0].departAt, lastLeg.arriveAt),
        };
        const key = optionKey(option);
        if (!seen.has(key)) {
          seen.add(key);
          options.push(option);
        }
        continue;
      }

      if (legs.length - 1 >= config.maxTransfers) {
        continue;
      }

      const departures = await getStationSchedules(current.stationID);
      const candidateDepartures = (Array.isArray(departures) ? departures : [])
        .filter((schedule) => {
          const departAt = toTime(schedule.departs_at);
          const gap = departAt.getTime() - current.arriveAt.getTime();
          return gap >= config.minTransferMs && gap <= config.maxTransferMs;
        })
        .sort((a, b) => toTime(a.departs_at) - toTime(b.departs_at))
        .slice(0, config.maxCandidateDepartures);

      for (const next of candidateDepartures) {
        const nextRoute = await getRoute(next.train_id);
        if (!Array.isArray(nextRoute) || nextRoute.length === 0) {
          continue;
        }
        const nextDepartAt = toTime(next.departs_at);
        const boardIdx = findStopIndex(nextRoute, current.stationID, new Date(nextDepartAt.getTime() - config.minTransferMs));
        if (boardIdx < 0) {
          continue;
        }
        const maxForwardStops = Math.min(nextRoute.length, boardIdx + config.maxForwardStops);
        for (let i = boardIdx + 1; i < maxForwardStops; i++) {
          const nextStation = String(nextRoute[i].station_id || "").toUpperCase();
          if (!nextStation || current.visited.has(nextStation)) {
            continue;
          }
          const nextArriveAt = getStopTime(nextRoute[i]);
          const nextLeg = {
            trainId: next.train_id,
            line: next.line,
            from: current.stationID,
            to: nextStation,
            departAt: nextDepartAt,
            arriveAt: nextArriveAt,
          };
          const nextVisited = new Set(current.visited);
          nextVisited.add(nextStation);
          queue.push({
            stationID: nextStation,
            arriveAt: nextArriveAt,
            legs: [...legs, nextLeg],
            visited: nextVisited,
          });
          if (queue.length >= config.maxQueueSize) {
            truncated = true;
            break;
          }
        }
        if (queue.length >= config.maxQueueSize) {
          truncated = true;
          break;
        }
      }
    }

    return {
      options: finalizeOptions(options, config.maxResults),
      stats: { expandedStates, truncated },
    };
  }

  const api = { findTripOptions };
  if (typeof module !== "undefined" && module.exports) {
    module.exports = api;
  }
  root.PlannerCore = api;
})(typeof window !== "undefined" ? window : globalThis);
