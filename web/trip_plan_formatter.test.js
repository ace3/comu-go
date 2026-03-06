const test = require("node:test");
const assert = require("node:assert/strict");

const {
  buildLegDetailText,
  buildTransferDetailText,
  classifyTransferWait,
  findAlternateDeparture,
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
        return [{ station_id: "DU" }, { station_id: "AK" }, { station_id: "BKS" }];
      }
      if (trainID === "5064B") {
        return [{ station_id: "DU" }, { station_id: "SUD" }, { station_id: "CKR" }];
      }
      return [];
    },
  );

  assert.ok(alternate);
  assert.equal(alternate.train_id, "5064B");
});
