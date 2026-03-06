(function (root) {
  const topologyApi = root.KRLTopology || (typeof require === "function" ? (() => {
    try {
      return require("./krl_topology.js");
    } catch (_) {
      return null;
    }
  })() : null);

  function diffMinutes(from, to) {
    return Math.max(0, Math.round((to.getTime() - from.getTime()) / 60000));
  }

  function classifyTransferWait(waitMin) {
    if (waitMin <= 4) {
      return "tight transfer";
    }
    if (waitMin >= 25) {
      return "long wait";
    }
    return "";
  }

  function buildLegDetailText(leg, formatTime, stationNameOnly) {
    return `${stationNameOnly(leg.from)} dep ${formatTime(leg.departAt)} • ${stationNameOnly(leg.to)} arr ${formatTime(leg.arriveAt)}`;
  }

  function buildTransferDetailText(firstLeg, secondLeg, formatTime, stationNameOnly) {
    const waitMin = diffMinutes(firstLeg.arriveAt, secondLeg.departAt);
    const classification = classifyTransferWait(waitMin);
    const suffix = classification ? ` (${classification})` : "";
    return `Transit at ${stationNameOnly(firstLeg.to)} • arrive ${formatTime(firstLeg.arriveAt)} • depart ${formatTime(secondLeg.departAt)} • wait ${waitMin} min${suffix}`;
  }

  function optionHasLongWait(option) {
    if (!option || !Array.isArray(option.legs) || option.legs.length < 2) {
      return false;
    }
    for (let i = 0; i < option.legs.length - 1; i++) {
      const first = option.legs[i];
      const second = option.legs[i + 1];
      const waitMin = diffMinutes(first.arriveAt, second.departAt);
      if (classifyTransferWait(waitMin) === "long wait") {
        return true;
      }
    }
    return false;
  }

  function scheduleDepartsAfterCurrent(schedule, currentLeg) {
    if (!schedule || !currentLeg) {
      return false;
    }
    const dep = new Date(schedule.departs_at);
    return String(schedule.line || "") === String(currentLeg.line || "") && dep > currentLeg.departAt;
  }

  function parseStopTime(stop) {
    if (!stop) {
      return null;
    }
    const raw = stop.departs_at || stop.arrives_at;
    if (!raw) {
      return null;
    }
    const value = new Date(raw);
    return Number.isNaN(value.getTime()) ? null : value;
  }

  function findArrivalStopAfterDeparture(route, destStationID, boardedDepartureAt) {
    if (!Array.isArray(route) || !destStationID || !boardedDepartureAt) {
      return null;
    }

    const destination = String(destStationID || "").toUpperCase();
    const boardedAt = boardedDepartureAt instanceof Date ? boardedDepartureAt : new Date(boardedDepartureAt);
    if (Number.isNaN(boardedAt.getTime())) {
      return null;
    }

    let bestStop = null;
    let bestTime = null;
    for (const stop of route) {
      if (String(stop.station_id || "").toUpperCase() !== destination) {
        continue;
      }
      const stopTime = parseStopTime(stop);
      if (!stopTime || stopTime <= boardedAt) {
        continue;
      }
      if (!bestTime || stopTime < bestTime) {
        bestStop = stop;
        bestTime = stopTime;
      }
    }
    return bestStop;
  }

  async function findAlternateDeparture(schedules, currentLeg, destStationID, fetchRoute, options = {}) {
    if (!Array.isArray(schedules) || !currentLeg || !destStationID || typeof fetchRoute !== "function") {
      return null;
    }

    const topology = options?.topology || null;
    const candidates = schedules
      .filter((schedule) => scheduleDepartsAfterCurrent(schedule, currentLeg))
      .sort((a, b) => new Date(a.departs_at) - new Date(b.departs_at));

    const destination = String(destStationID || "").toUpperCase();
    for (const candidate of candidates) {
      if (topology && topologyApi) {
        const classification = topologyApi.classifyRoute(
          topology,
          candidate.route,
          [],
        );
        if (classification?.ok && !topologyApi.canReach(topology, classification.corridorID, currentLeg.from, destination)) {
          continue;
        }
      }
      let route;
      try {
        route = await fetchRoute(candidate.train_id);
      } catch (_) {
        continue;
      }
      const destinationStop = findArrivalStopAfterDeparture(route, destination, new Date(candidate.departs_at));
      if (destinationStop) {
        return candidate;
      }
    }

    return null;
  }

  const api = {
    buildLegDetailText,
    buildTransferDetailText,
    classifyTransferWait,
    findAlternateDeparture,
    findArrivalStopAfterDeparture,
    optionHasLongWait,
  };

  if (typeof module !== "undefined" && module.exports) {
    module.exports = api;
  }
  root.TripPlanFormatter = api;
})(typeof window !== "undefined" ? window : globalThis);
