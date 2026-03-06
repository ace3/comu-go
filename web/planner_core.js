(function (root) {
  const topologyApi = (() => {
    if (root.KRLTopology) {
      return root.KRLTopology;
    }
    if (typeof require === "function") {
      try {
        return require("./krl_topology.js");
      } catch (_) {
        return null;
      }
    }
    return null;
  })();

  const DEFAULTS = {
    minTransferMs: 1 * 60 * 1000,
    maxTransferMs: 240 * 60 * 1000,
    maxTransfers: 3,
    maxCandidateDepartures: 60,
    maxForwardStops: 30,
    maxQueueSize: 1200,
    maxExpandedStates: 1500,
    maxRuntimeMs: 7000,
    maxResults: 8,
    lookbackMs: 10 * 60 * 1000,
    lookaheadMs: 6 * 60 * 60 * 1000,
    targetTransferWaitMin: 10,
  };

  const TERMINAL_CODE_BY_NAME = {
    TANGERANG: "TNG",
    DURI: "DU",
    MANGGARAI: "MRI",
    TANAHABANG: "THB",
    SUDIRMANBARU: "SUDB",
    SUDIRMAN: "SUD",
    KARET: "KAT",
    BOGOR: "BOO",
    JAKARTAKOTA: "JAKK",
    KAMPUNGBANDAN: "KPB",
    CIKARANG: "CKR",
    BEKASI: "BKS",
    ANGKE: "AK",
    TAMBUN: "TB",
    BANDARSOEKARNOHATTA: "BST",
    SOEKARNOHATTA: "BST",
  };

  function toTime(value) {
    return value instanceof Date ? value : new Date(value);
  }

  function defaultTopology() {
    if (typeof require === "function") {
      try {
        return require("./krl_topology.json");
      } catch (_) {
        return null;
      }
    }
    return root.__KRL_TOPOLOGY_DATA__ || null;
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

    // Primary ordering: fastest arrival first.
    const arriveDiff = a.arriveAt - b.arriveAt;
    if (arriveDiff !== 0) {
      return arriveDiff;
    }

    const waitRankDiff = transferWaitRank(a) - transferWaitRank(b);
    if (waitRankDiff !== 0) {
      return waitRankDiff;
    }

    const waitTargetDiff = transferTargetDelta(a) - transferTargetDelta(b);
    if (waitTargetDiff !== 0) {
      return waitTargetDiff;
    }

    const waitMinutesDiff = transferWaitMinutes(a) - transferWaitMinutes(b);
    if (waitMinutesDiff !== 0) {
      return waitMinutesDiff;
    }
    const departDiff = b.departAt - a.departAt;
    if (departDiff !== 0) {
      return departDiff;
    }

    return a.durationMinutes - b.durationMinutes;
  }

  function transferWaitMinutes(option) {
    if (!option || !Array.isArray(option.legs) || option.legs.length < 2) {
      return 0;
    }
    const first = option.legs[0];
    const second = option.legs[1];
    return diffMinutes(first.arriveAt, second.departAt);
  }

  function transferWaitRank(option) {
    if (!option || !Array.isArray(option.legs) || option.legs.length < 2) {
      return 0;
    }
    const waitMin = transferWaitMinutes(option);
    if (waitMin < 5) {
      return 2; // Too tight.
    }
    if (waitMin > 25) {
      return 1; // Too long.
    }
    return 0; // Safe range.
  }

  function transferTargetDelta(option) {
    if (!option || !Array.isArray(option.legs) || option.legs.length < 2) {
      return 0;
    }
    return Math.abs(transferWaitMinutes(option) - DEFAULTS.targetTransferWaitMin);
  }

  function finalizeOptions(options, maxResults) {
    if (!Array.isArray(options) || options.length === 0) {
      return [];
    }
    const optimalOnly = filterDominatedOptions(options);
    const sorted = [...optimalOnly].sort(sortOptions);
    const direct = sorted.filter((option) => option.legs.length === 1);
    if (direct.length > 0) {
      return direct.slice(0, maxResults);
    }
    const minLegs = sorted[0].legs.length;
    return sorted.filter((option) => option.legs.length === minLegs).slice(0, maxResults);
  }

  function isDominated(candidate, challenger) {
    if (!candidate || !challenger) {
      return false;
    }
    if (!Array.isArray(candidate.legs) || !Array.isArray(challenger.legs)) {
      return false;
    }
    if (candidate.legs.length !== challenger.legs.length || candidate.legs.length < 2) {
      return false;
    }
    const departsNoEarlier = challenger.departAt >= candidate.departAt;
    const arrivesStrictlyEarlier = challenger.arriveAt < candidate.arriveAt;
    return departsNoEarlier && arrivesStrictlyEarlier;
  }

  function filterDominatedOptions(options) {
    const kept = [];
    for (let i = 0; i < options.length; i++) {
      const candidate = options[i];
      let dominated = false;
      for (let j = 0; j < options.length; j++) {
        if (i === j) {
          continue;
        }
        if (isDominated(candidate, options[j])) {
          dominated = true;
          break;
        }
      }
      if (!dominated) {
        kept.push(candidate);
      }
    }
    return kept;
  }

  function pushUniqueOption(options, seen, option) {
    const key = optionKey(option);
    if (seen.has(key)) {
      return;
    }
    seen.add(key);
    options.push(option);
  }

  function normalizeStationToken(value) {
    return String(value || "").toUpperCase().replace(/[^A-Z0-9]/g, "");
  }

  function parseRouteTerminalID(routeName) {
    const route = String(routeName || "").trim();
    if (!route.includes("-")) {
      return "";
    }
    const parts = route.split("-").map((part) => normalizeStationToken(part)).filter(Boolean);
    if (parts.length < 2) {
      return "";
    }
    const raw = parts[parts.length - 1];
    return TERMINAL_CODE_BY_NAME[raw] || "";
  }

  function extractForwardStops(route, fromID, minTime, maxForwardStops, routeName, departAt) {
    if (!Array.isArray(route) || route.length === 0) {
      return [];
    }
    const fromIdx = findStopIndex(route, fromID, minTime);
    if (fromIdx < 0) {
      return [];
    }
    const maxForward = Math.min(route.length, fromIdx + maxForwardStops);
    const forwardStops = route.slice(fromIdx + 1, maxForward).map((stop) => ({
      stationID: String(stop.station_id || "").toUpperCase(),
      arriveAt: getStopTime(stop),
    })).filter((stop) => stop.stationID && stop.stationID !== fromID);

    const terminalID = parseRouteTerminalID(routeName);
    if (terminalID && terminalID !== fromID && !forwardStops.some((stop) => stop.stationID === terminalID)) {
      const tailTime = forwardStops.length > 0
        ? forwardStops[forwardStops.length - 1].arriveAt
        : toTime(departAt);
      forwardStops.push({
        stationID: terminalID,
        arriveAt: new Date(tailTime.getTime() + (5 * 60 * 1000)),
      });
    }

    return forwardStops;
  }

  function findStopIndexInRoute(route, stationID) {
    return route.findIndex((stop) => String(stop.station_id || "").toUpperCase() === stationID);
  }

  function interpolateStops(prev, next, stations, routeName) {
    if (!Array.isArray(stations) || stations.length === 0) {
      return [];
    }
    const prevTime = getStopTime(prev);
    const nextTime = getStopTime(next);
    let stepMs = 60 * 1000;
    if (nextTime > prevTime) {
      stepMs = Math.max(60 * 1000, Math.round((nextTime.getTime() - prevTime.getTime()) / (stations.length + 1)));
    }

    return stations.map((stationID, index) => {
      const at = new Date(prevTime.getTime() + (stepMs * (index + 1)));
      return {
        station_id: stationID,
        departs_at: at.toISOString(),
        arrives_at: at.toISOString(),
        route: routeName,
      };
    });
  }

  function appendGraphStops(route, fromID, routeName, topology) {
    if (!Array.isArray(route) || route.length === 0 || !topology || !topologyApi) {
      return route;
    }
    const stops = route.map((stop) => String(stop.station_id || "").toUpperCase());
    const classification = topologyApi.classifyRoute(topology, routeName, stops);
    if (!classification?.ok) {
      return route;
    }

    const corridorStations = topology.corridors?.[classification.corridorID] || [];
    const corridorIndex = {};
    corridorStations.forEach((stationID, index) => {
      corridorIndex[String(stationID || "").toUpperCase()] = index;
    });

    const points = route
      .map((stop, routeIdx) => ({
        routeIdx,
        corridorIdx: corridorIndex[String(stop.station_id || "").toUpperCase()],
      }))
      .filter((point) => Number.isInteger(point.corridorIdx));
    if (points.length === 0) {
      return route;
    }

    let updated = [...route];
    let inserted = 0;
    for (let i = 0; i < points.length - 1; i += 1) {
      const left = points[i];
      const right = points[i + 1];
      if (right.corridorIdx - left.corridorIdx <= 1) {
        continue;
      }
      const insertAt = right.routeIdx + inserted;
      const missingStations = corridorStations.slice(left.corridorIdx + 1, right.corridorIdx);
      const fillerStops = interpolateStops(updated[insertAt - 1], updated[insertAt], missingStations, routeName);
      updated = [
        ...updated.slice(0, insertAt),
        ...fillerStops,
        ...updated.slice(insertAt),
      ];
      inserted += fillerStops.length;
    }

    const terminalID = parseRouteTerminalID(routeName);
    if (!terminalID || findStopIndexInRoute(updated, terminalID) >= 0) {
      return updated;
    }

    const lastKnown = [...updated].reverse().find((stop) => Number.isInteger(corridorIndex[String(stop.station_id || "").toUpperCase()]));
    if (!lastKnown) {
      return updated;
    }
    const lastIdx = corridorIndex[String(lastKnown.station_id || "").toUpperCase()];
    const terminalIdx = corridorIndex[terminalID];
    if (!Number.isInteger(terminalIdx) || terminalIdx <= lastIdx) {
      return updated;
    }
    const tailTime = getStopTime(updated[updated.length - 1]);
    const at = new Date(tailTime.getTime() + (5 * 60 * 1000));
    return updated.concat({
      station_id: terminalID,
      departs_at: at.toISOString(),
      arrives_at: at.toISOString(),
      route: routeName,
    });
  }

  function prepareRoute(route, fromID, routeName, plannerMode, topology) {
    if (plannerMode === "graph") {
      return appendGraphStops(route, fromID, routeName, topology);
    }
    return route;
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
    const plannerMode = String(input?.plannerMode || "legacy").toLowerCase() === "graph" ? "graph" : "legacy";
    const topology = input?.topology || defaultTopology();
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

    const options = [];
    const seen = new Set();
    let truncated = false;
    let expandedStates = 0;
    const startedAt = Date.now();
    const oneTransferSeeds = new Map();

    for (const first of firstLegCandidates) {
      const departAt = toTime(first.departs_at);
      const route = prepareRoute(await getRoute(first.train_id), fromID, first.route, plannerMode, topology);
      const forwardStops = extractForwardStops(
        route,
        fromID,
        new Date(departAt.getTime() - config.minTransferMs),
        config.maxForwardStops,
        first.route,
        departAt,
      );
      for (const stop of forwardStops) {
        expandedStates += 1;
        if (Date.now() - startedAt > config.maxRuntimeMs || expandedStates >= config.maxExpandedStates) {
          truncated = true;
          break;
        }
        const firstLeg = {
          trainId: first.train_id,
          line: first.line,
          from: fromID,
          to: stop.stationID,
          departAt,
          arriveAt: stop.arriveAt,
        };

        if (stop.stationID === toID) {
          pushUniqueOption(options, seen, {
            legs: [firstLeg],
            departAt: firstLeg.departAt,
            arriveAt: firstLeg.arriveAt,
            durationMinutes: diffMinutes(firstLeg.departAt, firstLeg.arriveAt),
          });
          continue;
        }

        if (plannerMode === "graph" && topologyApi && topology && !topologyApi.isInterchange(topology, stop.stationID)) {
          continue;
        }
        if (!oneTransferSeeds.has(stop.stationID)) {
          oneTransferSeeds.set(stop.stationID, []);
        }
        const seedBucket = oneTransferSeeds.get(stop.stationID);
        if (seedBucket.length < 6) {
          seedBucket.push(firstLeg);
        }
      }
      if (truncated) {
        break;
      }
    }

    const directOnly = finalizeOptions(options, config.maxResults);
    if (directOnly.length > 0) {
      return {
        options: directOnly,
        stats: { expandedStates, truncated },
      };
    }

    const oneTransferOptions = [];
    const oneTransferSeen = new Set();
    for (const [transferStationID, firstLegs] of oneTransferSeeds.entries()) {
      if (Date.now() - startedAt > config.maxRuntimeMs || expandedStates >= config.maxExpandedStates) {
        truncated = true;
        break;
      }
      const departures = await getStationSchedules(transferStationID);
      const candidateDepartures = (Array.isArray(departures) ? departures : [])
        .filter((schedule) => {
          const secondDepart = toTime(schedule.departs_at);
          const earliestArrive = firstLegs.reduce((min, leg) => (leg.arriveAt < min ? leg.arriveAt : min), firstLegs[0].arriveAt);
          const gap = secondDepart.getTime() - earliestArrive.getTime();
          return gap >= config.minTransferMs && gap <= config.maxTransferMs;
        })
        .sort((a, b) => toTime(a.departs_at) - toTime(b.departs_at))
        .slice(0, config.maxCandidateDepartures);

      for (const second of candidateDepartures) {
        expandedStates += 1;
        if (Date.now() - startedAt > config.maxRuntimeMs || expandedStates >= config.maxExpandedStates) {
          truncated = true;
          break;
        }
        const secondDepartAt = toTime(second.departs_at);
        const secondRoute = prepareRoute(await getRoute(second.train_id), transferStationID, second.route, plannerMode, topology);
        if (plannerMode === "graph" && topologyApi && topology) {
          const classification = topologyApi.classifyRoute(
            topology,
            second.route,
            secondRoute.map((stop) => stop.station_id),
          );
          if (classification?.ok && !topologyApi.canReach(topology, classification.corridorID, transferStationID, toID)) {
            if (!secondRoute.some((stop) => String(stop.station_id || "").toUpperCase() === toID)) {
              continue;
            }
          }
        }
        const secondForwardStops = extractForwardStops(
          secondRoute,
          transferStationID,
          new Date(secondDepartAt.getTime() - config.minTransferMs),
          config.maxForwardStops,
          second.route,
          secondDepartAt,
        );
        const toStop = secondForwardStops.find((stop) => stop.stationID === toID);
        if (!toStop) {
          continue;
        }

        for (const firstLeg of firstLegs) {
          const transferGap = secondDepartAt.getTime() - firstLeg.arriveAt.getTime();
          if (transferGap < config.minTransferMs || transferGap > config.maxTransferMs) {
            continue;
          }
          const secondLeg = {
            trainId: second.train_id,
            line: second.line,
            from: transferStationID,
            to: toID,
            departAt: secondDepartAt,
            arriveAt: toStop.arriveAt,
          };
          pushUniqueOption(oneTransferOptions, oneTransferSeen, {
            legs: [firstLeg, secondLeg],
            departAt: firstLeg.departAt,
            arriveAt: secondLeg.arriveAt,
            durationMinutes: diffMinutes(firstLeg.departAt, secondLeg.arriveAt),
          });
          if (oneTransferOptions.length >= config.maxResults * 4) {
            break;
          }
        }
        if (oneTransferOptions.length >= config.maxResults * 4) {
          break;
        }
      }
      if (truncated || oneTransferOptions.length >= config.maxResults * 4) {
        break;
      }
    }

    return {
      options: finalizeOptions(oneTransferOptions, config.maxResults),
      stats: { expandedStates, truncated },
    };
  }

  const api = { findTripOptions };
  if (typeof module !== "undefined" && module.exports) {
    module.exports = api;
  }
  root.PlannerCore = api;
})(typeof window !== "undefined" ? window : globalThis);
