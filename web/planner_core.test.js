const test = require("node:test");
const assert = require("node:assert/strict");
const { findTripOptions } = require("./planner_core.js");

function iso(hhmm) {
  return `2026-03-05T${hhmm}:00+07:00`;
}

function stop(stationID, hhmm) {
  return { station_id: stationID, departs_at: iso(hhmm), arrives_at: iso(hhmm) };
}

const routesByTrainID = {
  A1: [
    stop("TNG", "14:00"),
    stop("THI", "14:05"),
    stop("BCP", "14:08"),
    stop("PI", "14:11"),
    stop("KDS", "14:14"),
    stop("RW", "14:18"),
    stop("BOI", "14:22"),
    stop("TKO", "14:25"),
    stop("PSG", "14:28"),
    stop("GRG", "14:31"),
    stop("DU", "14:35"),
  ],
  A2: [
    stop("TNG", "14:07"),
    stop("THI", "14:12"),
    stop("BCP", "14:15"),
    stop("PI", "14:18"),
    stop("KDS", "14:21"),
    stop("RW", "14:25"),
  ],
  AP1: [
    stop("DU", "14:42"),
    stop("BNC", "14:47"),
    stop("SUDB", "14:52"),
  ],
};

const schedulesByStation = {
  TNG: [
    { train_id: "A2", line: "Commuter Line Tangerang", departs_at: iso("14:07") },
    { train_id: "A1", line: "Commuter Line Tangerang", departs_at: iso("14:00") },
  ],
  RW: [
    { train_id: "A1", line: "Commuter Line Tangerang", departs_at: iso("14:18") },
    { train_id: "A2", line: "Commuter Line Tangerang", departs_at: iso("14:25") },
  ],
  DU: [
    { train_id: "AP1", line: "Commuter Line Basoetta", departs_at: iso("14:42") },
  ],
};

async function getRoute(trainID) {
  return routesByTrainID[trainID] || [];
}

async function getStationSchedules(stationID) {
  return schedulesByStation[stationID] || [];
}

test("Tangerang -> Rawa Buaya returns direct options only", async () => {
  const result = await findTripOptions({
    fromID: "TNG",
    toID: "RW",
    now: new Date(iso("13:55")),
    firstLegSchedules: schedulesByStation.TNG,
    getRoute,
    getStationSchedules,
  });

  assert.ok(result.options.length > 0);
  assert.ok(result.options.every((option) => option.legs.length === 1));
});

test("Rawa Buaya -> Duri returns direct route", async () => {
  const result = await findTripOptions({
    fromID: "RW",
    toID: "DU",
    now: new Date(iso("14:00")),
    firstLegSchedules: schedulesByStation.RW,
    getRoute,
    getStationSchedules,
  });

  assert.ok(result.options.length > 0);
  assert.ok(result.options.every((option) => option.legs.length === 1));
  assert.equal(result.options[0].legs[0].to, "DU");
});

test("Rawa Buaya -> Sudirman Baru returns one transfer at Duri", async () => {
  const result = await findTripOptions({
    fromID: "RW",
    toID: "SUDB",
    now: new Date(iso("14:00")),
    firstLegSchedules: schedulesByStation.RW,
    getRoute,
    getStationSchedules,
  });

  assert.ok(result.options.length > 0);
  assert.ok(result.options.every((option) => option.legs.length === 2));
  const first = result.options[0];
  assert.equal(first.legs[0].to, "DU");
  assert.equal(first.legs[1].from, "DU");
  assert.equal(first.legs[1].to, "SUDB");
});

test("one-transfer options are ordered by fastest arrival first", async () => {
  const customRoutesByTrainID = {
    F1: [
      stop("RW", "14:00"),
      stop("DU", "14:10"),
    ],
    F2: [
      stop("RW", "14:20"),
      stop("DU", "14:30"),
    ],
    TIGHT: [
      stop("DU", "14:32"),
      stop("SUDB", "14:50"),
    ],
    SAFE: [
      stop("DU", "14:38"),
      stop("SUDB", "14:55"),
    ],
    LONG: [
      stop("DU", "15:10"),
      stop("SUDB", "15:25"),
    ],
  };

  const customSchedulesByStation = {
    RW: [
      { train_id: "F1", line: "Commuter Line Tangerang", departs_at: iso("14:00") },
      { train_id: "F2", line: "Commuter Line Tangerang", departs_at: iso("14:20") },
    ],
    DU: [
      { train_id: "TIGHT", line: "Commuter Line Cikarang", departs_at: iso("14:32") },
      { train_id: "SAFE", line: "Commuter Line Cikarang", departs_at: iso("14:38") },
      { train_id: "LONG", line: "Commuter Line Cikarang", departs_at: iso("15:10") },
    ],
  };

  async function customGetRoute(trainID) {
    return customRoutesByTrainID[trainID] || [];
  }

  async function customGetStationSchedules(stationID) {
    return customSchedulesByStation[stationID] || [];
  }

  const result = await findTripOptions({
    fromID: "RW",
    toID: "SUDB",
    now: new Date(iso("13:50")),
    firstLegSchedules: customSchedulesByStation.RW,
    getRoute: customGetRoute,
    getStationSchedules: customGetStationSchedules,
    config: { maxResults: 6 },
  });

  assert.equal(result.options.length, 2);
  // F1→TIGHT (22min safe wait) ranks above F2→TIGHT (2min tight wait) by wait quality
  assert.equal(result.options[0].legs[0].trainId, "F1");
  assert.equal(result.options[0].legs[1].trainId, "TIGHT");
  assert.equal(result.options[1].legs[0].trainId, "F2");
  assert.equal(result.options[1].legs[1].trainId, "TIGHT");
});

test("dominates and removes non-optimal options across different departure times", async () => {
  const customRoutesByTrainID = {
    F1: [
      stop("RW", "16:05"),
      stop("DU", "16:21"),
    ],
    F2: [
      stop("RW", "16:22"),
      stop("DU", "16:38"),
    ],
    FAST: [
      stop("DU", "16:31"),
      stop("SUDB", "17:18"),
    ],
    GOOD: [
      stop("DU", "16:31"),
      stop("SUDB", "17:46"),
    ],
    SLOW: [
      stop("DU", "16:41"),
      stop("SUDB", "18:05"),
    ],
  };

  const customSchedulesByStation = {
    RW: [
      { train_id: "F1", line: "Commuter Line Tangerang", departs_at: iso("16:05") },
      { train_id: "F2", line: "Commuter Line Tangerang", departs_at: iso("16:22") },
    ],
    DU: [
      { train_id: "FAST", line: "Commuter Line Cikarang", departs_at: iso("16:31") },
      { train_id: "GOOD", line: "Commuter Line Cikarang", departs_at: iso("16:31") },
      { train_id: "SLOW", line: "Commuter Line Cikarang", departs_at: iso("16:41") },
    ],
  };

  const result = await findTripOptions({
    fromID: "RW",
    toID: "SUDB",
    now: new Date(iso("15:50")),
    firstLegSchedules: customSchedulesByStation.RW,
    getRoute: async (trainID) => customRoutesByTrainID[trainID] || [],
    getStationSchedules: async (stationID) => customSchedulesByStation[stationID] || [],
    config: { maxResults: 8 },
  });

  const hasDominatedSlowOption = result.options.some(
    (option) => option.legs[0].trainId === "F1" && option.legs[1].trainId === "SLOW",
  );
  assert.equal(hasDominatedSlowOption, false);

  const hasEarlyDepartAndLaterArrival = result.options.some(
    (option) => option.legs[0].trainId === "F1" && option.legs[1].trainId === "SLOW",
  );
  assert.equal(hasEarlyDepartAndLaterArrival, false);
});

test("graph planner infers valid transfer stop from topology when route data is partial", async () => {
  const customRoutesByTrainID = {
    F1: [
      stop("SUD", "20:20"),
      stop("THB", "20:29"),
      stop("AK", "20:40"),
      stop("KPB", "20:47"),
    ],
    T1: [
      stop("DU", "20:50"),
      stop("RW", "21:05"),
    ],
  };

  const customSchedulesByStation = {
    SUD: [
      {
        train_id: "F1",
        line: "Commuter Line Cikarang",
        route: "CIKARANG-KAMPUNGBANDAN VIA MRI",
        departs_at: iso("20:20"),
      },
    ],
    DU: [
      {
        train_id: "T1",
        line: "Commuter Line Tangerang",
        route: "DU-RW",
        departs_at: iso("20:50"),
      },
    ],
  };

  const result = await findTripOptions({
    fromID: "SUD",
    toID: "RW",
    now: new Date(iso("20:05")),
    firstLegSchedules: customSchedulesByStation.SUD,
    getRoute: async (trainID) => customRoutesByTrainID[trainID] || [],
    getStationSchedules: async (stationID) => customSchedulesByStation[stationID] || [],
    plannerMode: "graph",
  });

  assert.ok(result.options.length > 0);
  assert.ok(result.options.some((option) =>
    option.legs.length === 2
    && option.legs[0].trainId === "F1"
    && option.legs[0].to === "DU"
    && option.legs[1].trainId === "T1"));
});
