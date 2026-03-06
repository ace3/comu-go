const test = require("node:test");
const assert = require("node:assert/strict");

const {
  buildLegDetailText,
  buildTransferDetailText,
  classifyTransferWait,
  findAlternateDeparture,
  findArrivalStopAfterDeparture,
  optionHasLongWait,
} = require("./trip_plan_formatter.js");

function at(hhmm) {
  return new Date(`2026-03-05T${hhmm}:00+07:00`);
}

function timeLabel(date) {
  return new Intl.DateTimeFormat("id-ID", {
    timeZone: "Asia/Jakarta",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(date);
}

test("buildLegDetailText includes departure and arrival times", () => {
  const leg = {
    from: "RW",
    to: "DU",
    departAt: at("15:22"),
    arriveAt: at("15:35"),
  };

  const detail = buildLegDetailText(leg, timeLabel, (stationID) => stationID);
  assert.equal(detail, "RW dep 15.22 • DU arr 15.35");
});

test("buildTransferDetailText includes arrive/depart/wait at transfer station", () => {
  const firstLeg = { to: "DU", arriveAt: at("15:35") };
  const secondLeg = { from: "DU", departAt: at("16:43") };

  const detail = buildTransferDetailText(firstLeg, secondLeg, timeLabel, (stationID) => stationID);
  assert.equal(detail, "Transit at DU • arrive 15.35 • depart 16.43 • wait 68 min (long wait)");
});

test("classifyTransferWait marks short transfer as tight", () => {
  assert.equal(classifyTransferWait(3), "tight transfer");
  assert.equal(classifyTransferWait(8), "");
  assert.equal(classifyTransferWait(55), "long wait");
});

test("optionHasLongWait detects long transfer waits", () => {
  const longWaitOption = {
    legs: [
      { arriveAt: at("15:35") },
      { departAt: at("16:43") },
    ],
  };
  const safeOption = {
    legs: [
      { arriveAt: at("15:35") },
      { departAt: at("15:48") },
    ],
  };

  assert.equal(optionHasLongWait(longWaitOption), true);
  assert.equal(optionHasLongWait(safeOption), false);
});

test("findAlternateDeparture skips same-line trains that do not reach the destination", async () => {
  const schedules = [
    {
      train_id: "5080B",
      line: "Commuter Line Cikarang",
      departs_at: "2026-03-06T11:07:00+07:00",
      route: "ANGKE-BEKASI",
    },
    {
      train_id: "5064B",
      line: "Commuter Line Cikarang",
      departs_at: "2026-03-06T09:32:00+07:00",
      route: "ANGKE-CIKARANG",
    },
  ];
  const currentLeg = {
    trainId: "5062C",
    line: "Commuter Line Cikarang",
    departAt: new Date("2026-03-06T09:22:00+07:00"),
  };

  const alternate = await findAlternateDeparture(
    schedules,
    currentLeg,
    "SUD",
    async (trainID) => {
      if (trainID === "5080B") {
        return [
          { station_id: "DU", departs_at: "2026-03-06T11:07:00+07:00" },
          { station_id: "AK", departs_at: "2026-03-06T11:17:00+07:00" },
          { station_id: "BKS", departs_at: "2026-03-06T11:29:00+07:00" },
        ];
      }
      if (trainID === "5064B") {
        return [
          { station_id: "DU", departs_at: "2026-03-06T09:32:00+07:00" },
          { station_id: "SUD", departs_at: "2026-03-06T09:41:00+07:00" },
          { station_id: "CKR", departs_at: "2026-03-06T10:18:00+07:00" },
        ];
      }
      return [];
    },
  );

  assert.ok(alternate);
  assert.equal(alternate.train_id, "5064B");
});

test("findAlternateDeparture skips trains where the destination stop is before the boarding stop", async () => {
  const schedules = [
    {
      train_id: "5089B",
      line: "Commuter Line Cikarang",
      departs_at: "2026-03-06T11:43:00+07:00",
      route: "BEKASI-ANGKE",
    },
    {
      train_id: "5086B",
      line: "Commuter Line Cikarang",
      departs_at: "2026-03-06T11:41:00+07:00",
      route: "ANGKE-CIKARANG",
    },
  ];
  const currentLeg = {
    trainId: "5086B",
    line: "Commuter Line Cikarang",
    from: "DU",
    departAt: new Date("2026-03-06T11:41:00+07:00"),
  };

  const alternate = await findAlternateDeparture(
    schedules,
    currentLeg,
    "SUDB",
    async (trainID) => {
      if (trainID === "5089B") {
        return [
          { station_id: "SUDB", departs_at: "2026-03-06T11:30:00+07:00" },
          { station_id: "DU", departs_at: "2026-03-06T11:43:00+07:00" },
          { station_id: "AK", departs_at: "2026-03-06T11:56:00+07:00" },
        ];
      }
      if (trainID === "5086B") {
        return [
          { station_id: "DU", departs_at: "2026-03-06T11:41:00+07:00" },
          { station_id: "SUDB", departs_at: "2026-03-06T11:50:00+07:00" },
        ];
      }
      return [];
    },
  );

  assert.equal(alternate, null);
});

test("findArrivalStopAfterDeparture returns only a destination stop after boarded departure", () => {
  const route = [
    { station_id: "SUDB", departs_at: "2026-03-06T11:30:00+07:00" },
    { station_id: "DU", departs_at: "2026-03-06T11:43:00+07:00" },
    { station_id: "SUDB", departs_at: "2026-03-06T11:58:00+07:00" },
  ];

  const beforeBoarding = findArrivalStopAfterDeparture(
    route,
    "SUDB",
    new Date("2026-03-06T11:43:00+07:00"),
  );

  assert.ok(beforeBoarding);
  assert.equal(new Date(beforeBoarding.departs_at).toISOString(), "2026-03-06T04:58:00.000Z");
});

test("findArrivalStopAfterDeparture projects snapshot route times onto the boarded date", () => {
  const route = [
    { station_id: "DU", departs_at: "2026-03-05T07:16:00Z" },
    { station_id: "THB", departs_at: "2026-03-05T07:23:00Z" },
    { station_id: "SUDB", departs_at: "2026-03-05T07:26:30Z" },
  ];

  const stop = findArrivalStopAfterDeparture(
    route,
    "SUDB",
    new Date("2026-03-06T14:09:00+07:00"),
  );

  assert.ok(stop);
  assert.equal(stop.station_id, "SUDB");
  assert.equal(new Date(stop.departs_at).toISOString(), "2026-03-06T07:26:30.000Z");
});
