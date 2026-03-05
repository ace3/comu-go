(function (root) {
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

  const api = {
    buildLegDetailText,
    buildTransferDetailText,
    classifyTransferWait,
    optionHasLongWait,
  };

  if (typeof module !== "undefined" && module.exports) {
    module.exports = api;
  }
  root.TripPlanFormatter = api;
})(typeof window !== "undefined" ? window : globalThis);
