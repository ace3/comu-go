const test = require("node:test");
const assert = require("node:assert/strict");

const {
  buildLegDetailText,
  buildTransferDetailText,
  classifyTransferWait,
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
