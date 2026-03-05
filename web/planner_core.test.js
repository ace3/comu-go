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

test("one-transfer options are ordered by nearest safe transfer before long/tight waits", async () => {
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

  assert.equal(result.options.length, 6);
  assert.equal(result.options[0].legs[0].trainId, "F2");
  assert.equal(result.options[0].legs[1].trainId, "SAFE");
  assert.equal(result.options[1].legs[0].trainId, "F1");
  assert.equal(result.options[1].legs[1].trainId, "TIGHT");
  assert.equal(result.options[2].legs[0].trainId, "F1");
  assert.equal(result.options[2].legs[1].trainId, "SAFE");
});
