(function (root) {
  function normalizeRoute(value) {
    return String(value || "").toUpperCase().replace(/[^A-Z0-9]/g, "");
  }

  function normalizeStationID(value) {
    return String(value || "").trim().toUpperCase();
  }

  function buildIndex(topology) {
    const corridors = topology?.corridors || {};
    const indexByCorridor = {};
    for (const [corridorID, stations] of Object.entries(corridors)) {
      const index = {};
      for (let i = 0; i < stations.length; i += 1) {
        index[normalizeStationID(stations[i])] = i;
      }
      indexByCorridor[corridorID] = index;
    }
    return indexByCorridor;
  }

  function isInterchange(topology, stationID) {
    const interchanges = topology?.interchanges || {};
    return Object.prototype.hasOwnProperty.call(interchanges, normalizeStationID(stationID));
  }

  function canReach(topology, corridorID, fromID, toID) {
    const indexByCorridor = buildIndex(topology);
    const index = indexByCorridor[corridorID];
    if (!index) {
      return false;
    }
    return Object.prototype.hasOwnProperty.call(index, normalizeStationID(fromID))
      && Object.prototype.hasOwnProperty.call(index, normalizeStationID(toID));
  }

  function classifyRoute(topology, routeName, stops) {
    const normalizedRoute = normalizeRoute(routeName);
    const hints = [
      ["VIAMRI", "cikarang_via_mri"],
      ["VIAPSE", "cikarang_via_pse"],
      ["TANGERANG", "tangerang"],
      ["NAMBO", "bogor_nambo"],
      ["MERAK", "merak"],
      ["RANGKASBITUNG", "rangkasbitung"],
      ["TANJUNGPRIOK", "tanjung_priok"],
      ["SOEKARNOHATTA", "airport"],
      ["BANDARA", "airport"],
    ];
    for (const [token, corridorID] of hints) {
      if (normalizedRoute.includes(token)) {
        return { corridorID, ok: true };
      }
    }

    const indexByCorridor = buildIndex(topology);
    const normalizedStops = Array.isArray(stops) ? stops.map(normalizeStationID).filter(Boolean) : [];
    const candidates = Object.entries(indexByCorridor)
      .filter(([, index]) => normalizedStops.every((stop) => Object.prototype.hasOwnProperty.call(index, stop)))
      .map(([corridorID]) => corridorID);

    if (candidates.length === 1) {
      return { corridorID: candidates[0], ok: true };
    }
    return { corridorID: "", ok: false };
  }

  const api = {
    canReach,
    classifyRoute,
    isInterchange,
    normalizeStationID,
  };

  if (typeof module !== "undefined" && module.exports) {
    module.exports = api;
  }
  root.KRLTopology = api;
})(typeof window !== "undefined" ? window : globalThis);
