const test = require("node:test");
const assert = require("node:assert/strict");
const { findTripOptions } = require("./planner_core.js");
const schedules = require("../data/schedules.json");

function buildIndexes() {
  const byStation = new Map();
  const byTrain = new Map();
  for (const schedule of schedules) {
    if (!byStation.has(schedule.station_id)) {
      byStation.set(schedule.station_id, []);
    }
    byStation.get(schedule.station_id).push(schedule);

    if (!byTrain.has(schedule.train_id)) {
      byTrain.set(schedule.train_id, []);
    }
    byTrain.get(schedule.train_id).push(schedule);
  }
  for (const values of byStation.values()) {
    values.sort((a, b) => new Date(a.departs_at) - new Date(b.departs_at));
  }
  for (const values of byTrain.values()) {
    values.sort((a, b) => new Date(a.departs_at) - new Date(b.departs_at));
  }
  return { byStation, byTrain };
}

const { byStation, byTrain } = buildIndexes();

async function getRoute(trainID) {
  return (byTrain.get(trainID) || []).map((schedule) => ({
    station_id: schedule.station_id,
    departs_at: schedule.departs_at,
    arrives_at: schedule.arrives_at,
  }));
}

async function getStationSchedules(stationID) {
  return byStation.get(stationID) || [];
}

test("real data: Rawa Buaya -> Duri returns direct options", async () => {
  const now = new Date("2026-03-05T14:09:52+07:00");
  const result = await findTripOptions({
    fromID: "RW",
    toID: "DU",
    now,
    firstLegSchedules: byStation.get("RW") || [],
    getRoute,
    getStationSchedules,
    config: { maxResults: 8 },
  });

  assert.ok(result.options.length > 0, "expected at least one option");
  assert.ok(result.options.every((option) => option.legs.length === 1), "expected direct-only results");
  assert.ok(result.options.every((option) => option.legs[0].to === "DU"), "expected destination DU");
});

test("real data: Rawa Buaya -> Sudirman Baru returns one-transfer options via Duri", async () => {
  const now = new Date("2026-03-05T14:09:52+07:00");
  const result = await findTripOptions({
    fromID: "RW",
    toID: "SUDB",
    now,
    firstLegSchedules: byStation.get("RW") || [],
    getRoute,
    getStationSchedules,
    config: { maxResults: 8 },
  });

  assert.ok(result.options.length > 0, "expected at least one option");
  assert.ok(result.options.every((option) => option.legs.length === 2), "expected one-transfer results only");
  assert.ok(result.options.every((option) => option.legs[0].to === "DU"), "expected first transfer station DU");
  assert.ok(result.options.every((option) => option.legs[1].from === "DU"), "expected second leg from DU");
  assert.ok(result.options.every((option) => option.legs[1].to === "SUDB"), "expected destination SUDB");
});
