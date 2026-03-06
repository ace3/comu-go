const STORAGE_KEY = "comu.preferred_stations";
const COOKIE_KEY = "preferred_stations";
const TRIP_PLANNER_PREFS_KEY = "comu.trip_planner_prefs";
const MAX_STATIONS = 5;
const WINDOW_BEFORE_MINUTES = 10;
const WINDOW_AFTER_MINUTES = 60;
const PLANNER_MAX_RESULTS = 8;
const TRIP_PLANNER_WINDOW_MINUTES = 60;
const WIB_ZONE = "Asia/Jakarta";
const API_BASE = "/v1";

const pickerView = document.getElementById("picker-view");
const scheduleView = document.getElementById("schedule-view");
const pickerStatusNode = document.getElementById("picker-status");
const stationSearch = document.getElementById("station-search");
const stationList = document.getElementById("station-list");
const saveButton = document.getElementById("save-stations");
const selectionCount = document.getElementById("selection-count");
const refreshButton = document.getElementById("refresh-data");
const editButton = document.getElementById("edit-stations");
const statusNode = document.getElementById("status");
const windowLabel = document.getElementById("window-label");
const cardsNode = document.getElementById("cards");
const planFrom = document.getElementById("plan-from");
const planTo = document.getElementById("plan-to");
const planSwap = document.getElementById("plan-swap");
const planShowAll = document.getElementById("plan-show-all");
const planShowLongWait = document.getElementById("plan-show-long-wait");
const planButton = document.getElementById("plan-trip");
const planStatusNode = document.getElementById("plan-status");
const planResultsNode = document.getElementById("plan-results");

let stations = [];
let preferredStations = [];
let selectedSet = new Set();
const routeCache = new Map();
const stationScheduleCache = new Map();
let topologyPromise = null;
let lastTripOptions = [];
let lastTripStats = null;
const tripPlanFormatter = window.TripPlanFormatter || {
  buildLegDetailText: (leg, formatTime, stationNameOnlyFn) =>
    `${stationNameOnlyFn(leg.from)} dep ${formatTime(leg.departAt)} • ${stationNameOnlyFn(leg.to)} arr ${formatTime(leg.arriveAt)}`,
  buildTransferDetailText: (firstLeg, secondLeg, formatTime, stationNameOnlyFn) => {
    const waitMin = Math.max(0, Math.round((secondLeg.departAt.getTime() - firstLeg.arriveAt.getTime()) / 60000));
    return `Transit at ${stationNameOnlyFn(firstLeg.to)} • arrive ${formatTime(firstLeg.arriveAt)} • depart ${formatTime(secondLeg.departAt)} • wait ${waitMin} min`;
  },
  classifyTransferWait: () => "",
  findAlternateDeparture: async (schedules, currentLeg) => {
    const candidates = Array.isArray(schedules)
      ? schedules
        .filter((schedule) => String(schedule.line || "") === String(currentLeg.line || "") && new Date(schedule.departs_at) > currentLeg.departAt)
        .sort((a, b) => new Date(a.departs_at) - new Date(b.departs_at))
      : [];
    return candidates[0] || null;
  },
  optionHasLongWait: () => false,
};

function sanitizeStationIDs(input) {
  if (!Array.isArray(input)) {
    return [];
  }

  const cleaned = [];
  const seen = new Set();
  for (const value of input) {
    const id = String(value || "").trim().toUpperCase();
    if (!id || seen.has(id)) {
      continue;
    }
    seen.add(id);
    cleaned.push(id);
    if (cleaned.length >= MAX_STATIONS) {
      break;
    }
  }
  return cleaned;
}

function setCookie(name, value, days) {
  const expires = new Date(Date.now() + days * 24 * 60 * 60 * 1000).toUTCString();
  document.cookie = `${name}=${encodeURIComponent(value)}; expires=${expires}; path=/; SameSite=Lax`;
}

function getCookie(name) {
  const parts = document.cookie.split(";").map((v) => v.trim());
  for (const part of parts) {
    if (part.startsWith(`${name}=`)) {
      return decodeURIComponent(part.slice(name.length + 1));
    }
  }
  return "";
}

function readPreferredStations() {
  try {
    const fromLocal = JSON.parse(localStorage.getItem(STORAGE_KEY) || "[]");
    const sanitizedLocal = sanitizeStationIDs(fromLocal);
    if (sanitizedLocal.length > 0) {
      return sanitizedLocal;
    }
  } catch (_) {
  }

  try {
    const cookieRaw = getCookie(COOKIE_KEY);
    const fromCookie = JSON.parse(cookieRaw || "[]");
    return sanitizeStationIDs(fromCookie);
  } catch (_) {
    return [];
  }
}

function savePreferredStations(stationIDs) {
  const sanitized = sanitizeStationIDs(stationIDs);
  preferredStations = sanitized;
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(sanitized));
  } catch (_) {
  }
  setCookie(COOKIE_KEY, JSON.stringify(sanitized), 30);
}

function readTripPlannerPrefs() {
  try {
    const raw = JSON.parse(localStorage.getItem(TRIP_PLANNER_PREFS_KEY) || "{}");
    return {
      fromID: String(raw.fromID || "").toUpperCase(),
      toID: String(raw.toID || "").toUpperCase(),
      plannerMode: normalizePlannerMode(raw.plannerMode),
      showLongWait: Boolean(raw.showLongWait),
    };
  } catch (_) {
    return { fromID: "", toID: "", plannerMode: "legacy", showLongWait: false };
  }
}

function saveTripPlannerPrefs(partial) {
  const current = readTripPlannerPrefs();
  const next = { ...current, ...partial };
  try {
    localStorage.setItem(TRIP_PLANNER_PREFS_KEY, JSON.stringify(next));
  } catch (_) {
  }
}

function normalizePlannerMode(value) {
  return String(value || "").toLowerCase() === "graph" ? "graph" : "legacy";
}

function plannerModeOverride() {
  try {
    const params = new URLSearchParams(window.location.search);
    return normalizePlannerMode(params.get("planner_mode") || params.get("planner"));
  } catch (_) {
    return "legacy";
  }
}

async function loadKRLTopology() {
  if (!topologyPromise) {
    topologyPromise = fetchWithTimeout("/app/assets/krl_topology.json");
  }
  return topologyPromise;
}

function showStatus(message, isError = false) {
  statusNode.textContent = message;
  statusNode.className = isError ? "mb-1 min-h-5 text-sm text-rose-600" : "mb-1 min-h-5 text-sm text-slate-600";
}

function showPickerStatus(message, isError = false) {
  pickerStatusNode.textContent = message;
  pickerStatusNode.className = isError ? "mb-3 mt-2 min-h-5 text-sm text-rose-600" : "mb-3 mt-2 min-h-5 text-sm text-slate-600";
}

function showPlanStatus(message, isError = false) {
  planStatusNode.textContent = message;
  planStatusNode.className = isError ? "mt-2 min-h-5 text-sm text-rose-600" : "mt-2 min-h-5 text-sm text-slate-600";
}

async function fetchWithTimeout(url, options = {}, timeoutMs = 10000) {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetch(url, {
      method: options.method || "GET",
      body: options.body,
      signal: controller.signal,
      headers: {
        Accept: "application/json",
        ...(options.headers || {}),
      },
    });
    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`);
    }
    return await response.json();
  } finally {
    clearTimeout(timeout);
  }
}

async function loadStations() {
  if (stations.length > 0) {
    return stations;
  }

  const payload = await fetchWithTimeout(`${API_BASE}/station?limit=500`);
  stations = (payload.data || [])
    .map((s) => ({ id: String(s.id || "").toUpperCase(), name: String(s.name || "") }))
    .filter((s) => s.id && s.name)
    .sort((a, b) => a.name.localeCompare(b.name));
  return stations;
}

function stationDisplayName(stationID) {
  const match = stations.find((s) => s.id === stationID);
  return match ? `${match.name} (${match.id})` : stationID;
}

function stationNameOnly(stationID) {
  const match = stations.find((s) => s.id === stationID);
  return match ? match.name : stationID;
}

function updateSelectionCount() {
  selectionCount.textContent = `${selectedSet.size} selected`;
  saveButton.disabled = selectedSet.size === 0 || selectedSet.size > MAX_STATIONS;
}

function renderStationPicker() {
  if (stations.length === 0) {
    stationList.innerHTML = '<p class="rounded-xl border border-slate-200 bg-slate-50 px-3 py-2 text-sm text-slate-600">No station data available yet. Run station sync first.</p>';
    saveButton.disabled = true;
    return;
  }

  const q = stationSearch.value.trim().toLowerCase();
  stationList.innerHTML = "";

  const filtered = stations.filter((s) => {
    if (!q) {
      return true;
    }
    return s.id.toLowerCase().includes(q) || s.name.toLowerCase().includes(q);
  });

  if (filtered.length === 0) {
    stationList.innerHTML = '<p class="rounded-xl border border-slate-200 bg-slate-50 px-3 py-2 text-sm text-slate-600">No station matches your search.</p>';
    return;
  }

  for (const station of filtered) {
    const wrapper = document.createElement("div");
    wrapper.className = "rounded-xl border border-slate-200 bg-white p-3 transition hover:border-cyan-300 hover:bg-cyan-50/40";

    const label = document.createElement("label");
    label.className = "flex min-h-11 cursor-pointer items-center gap-3";
    const checkbox = document.createElement("input");
    checkbox.type = "checkbox";
    checkbox.value = station.id;
    checkbox.checked = selectedSet.has(station.id);
    checkbox.className = "h-5 w-5 rounded border-slate-300 text-cyan-600 focus:ring-cyan-400";

    checkbox.addEventListener("change", () => {
      if (checkbox.checked) {
        if (selectedSet.size >= MAX_STATIONS) {
          checkbox.checked = false;
          showStatus(`Maximum ${MAX_STATIONS} stations only.`, true);
          return;
        }
        selectedSet.add(station.id);
      } else {
        selectedSet.delete(station.id);
      }
      updateSelectionCount();
    });

    const textWrap = document.createElement("div");
    textWrap.className = "min-w-0";
    const name = document.createElement("div");
    name.className = "truncate text-sm font-semibold text-slate-800";
    name.textContent = station.name;
    const id = document.createElement("div");
    id.className = "text-xs font-medium tracking-wide text-slate-500";
    id.textContent = station.id;

    textWrap.appendChild(name);
    textWrap.appendChild(id);
    label.appendChild(checkbox);
    label.appendChild(textWrap);
    wrapper.appendChild(label);
    stationList.appendChild(wrapper);
  }
}

function setView(mode) {
  const pick = mode === "picker";
  pickerView.classList.toggle("hidden", !pick);
  scheduleView.classList.toggle("hidden", pick);
}

function formatWIBTime(isoValue) {
  try {
    const date = new Date(isoValue);
    return new Intl.DateTimeFormat("id-ID", {
      timeZone: WIB_ZONE,
      hour: "2-digit",
      minute: "2-digit",
      hour12: false,
    }).format(date);
  } catch (_) {
    return "--:--";
  }
}

function titleCase(value) {
  return value
    .toLowerCase()
    .split(/\s+/)
    .filter(Boolean)
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(" ");
}

function parseDirection(schedule) {
  const route = String(schedule.route || "").trim();
  if (route.includes("-")) {
    const points = route
      .split("-")
      .map((part) => titleCase(part.replace(/[_\s]+/g, " ").trim()))
      .filter(Boolean);
    if (points.length >= 2) {
      return { from: points[0], to: points[points.length - 1] };
    }
  }

  const from = String(schedule.origin_id || "").trim().toUpperCase();
  const to = String(schedule.destination_id || "").trim().toUpperCase();
  return { from, to };
}

function buildWindowParams() {
  const windowMinutes = Math.ceil((WINDOW_BEFORE_MINUTES + WINDOW_AFTER_MINUTES) / 2);
  const atShiftMinutes = WINDOW_AFTER_MINUTES - windowMinutes;
  const at = new Date(Date.now() + atShiftMinutes * 60 * 1000);
  return { windowMinutes, at };
}

async function fetchTrainRoute(trainID) {
  if (routeCache.has(trainID)) {
    return routeCache.get(trainID);
  }
  const payload = await fetchWithTimeout(`${API_BASE}/route/${encodeURIComponent(trainID)}`);
  const route = payload?.data?.routes || [];
  routeCache.set(trainID, route);
  return route;
}

async function fetchStationSchedules(stationID) {
  if (stationScheduleCache.has(stationID)) {
    return stationScheduleCache.get(stationID);
  }
  const limit = 500;
  let page = 1;
  let total = 0;
  const schedules = [];

  while (true) {
    const payload = await fetchWithTimeout(
      `${API_BASE}/schedule/${encodeURIComponent(stationID)}?limit=${limit}&page=${page}`,
    );
    const items = Array.isArray(payload?.data) ? payload.data : [];
    schedules.push(...items);

    const metadata = payload?.metadata || {};
    total = Number(metadata.total || items.length || 0);
    if (items.length === 0 || schedules.length >= total || items.length < limit) {
      break;
    }
    page += 1;
  }

  stationScheduleCache.set(stationID, schedules);
  return schedules;
}

function diffMinutes(from, to) {
  return Math.max(0, Math.round((to.getTime() - from.getTime()) / 60000));
}

function yieldToBrowser() {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

function filterTripOptionsByDepartureWindow(options, now, windowMinutes) {
  const end = new Date(now.getTime() + (windowMinutes * 60 * 1000));
  return options.filter((option) => option.departAt >= now && option.departAt <= end);
}

function parseTripPlanOptions(rawOptions) {
  if (!Array.isArray(rawOptions)) {
    return [];
  }
  const parsed = [];
  for (const option of rawOptions) {
    const legs = Array.isArray(option.legs) ? option.legs : [];
    const normalizedLegs = [];
    for (const leg of legs) {
      const departAt = new Date(leg.departAt);
      const arriveAt = new Date(leg.arriveAt);
      if (Number.isNaN(departAt.getTime()) || Number.isNaN(arriveAt.getTime())) {
        continue;
      }
      normalizedLegs.push({
        trainId: String(leg.trainId || ""),
        line: String(leg.line || ""),
        from: String(leg.from || "").toUpperCase(),
        to: String(leg.to || "").toUpperCase(),
        departAt,
        arriveAt,
      });
    }
    if (normalizedLegs.length === 0) {
      continue;
    }
    const departAt = new Date(option.departAt);
    const arriveAt = new Date(option.arriveAt);
    if (Number.isNaN(departAt.getTime()) || Number.isNaN(arriveAt.getTime())) {
      continue;
    }
    parsed.push({
      legs: normalizedLegs,
      departAt,
      arriveAt,
      durationMinutes: Number(option.durationMinutes || diffMinutes(departAt, arriveAt)),
    });
  }
  return parsed;
}

function renderTripPlans(options) {
  planResultsNode.innerHTML = "";
  for (const option of options) {
    const card = document.createElement("article");
    const isTransit = option.legs.length > 1;
    card.className = isTransit
      ? "rounded-xl border border-amber-200 border-l-2 border-l-amber-400 bg-white p-3"
      : "rounded-xl border border-emerald-200 border-l-2 border-l-emerald-500 bg-white p-3";

    const head = document.createElement("div");
    head.className = "flex items-start justify-between gap-2";

    const mode = document.createElement("span");
    const transitCount = Math.max(0, option.legs.length - 1);
    mode.className = transitCount === 0
      ? "rounded-full bg-emerald-100 px-2 py-0.5 text-xs font-bold text-emerald-700"
      : "rounded-full bg-amber-100 px-2 py-0.5 text-xs font-bold text-amber-700";
    mode.textContent = transitCount === 0 ? "Direct" : `${transitCount} Transit`;

    const headRight = document.createElement("div");
    headRight.className = "text-right";
    const timing = document.createElement("div");
    timing.className = "text-sm font-bold text-slate-900";
    timing.textContent = `${formatWIBTime(option.departAt.toISOString())} → ${formatWIBTime(option.arriveAt.toISOString())}`;
    const duration = document.createElement("div");
    duration.className = "mt-0.5 text-xs text-slate-500";
    duration.textContent = `Duration ${option.durationMinutes} min`;
    headRight.appendChild(timing);
    headRight.appendChild(duration);

    head.appendChild(mode);
    head.appendChild(headRight);
    card.appendChild(head);

    const legs = document.createElement("div");
    legs.className = "mt-3 grid gap-2 text-sm text-slate-700";

    for (let i = 0; i < option.legs.length; i++) {
      const leg = option.legs[i];

      if (i > 0) {
        const divider = document.createElement("div");
        divider.className = "border-t border-dashed border-slate-100";
        legs.appendChild(divider);
      }

      const legNode = document.createElement("div");
      legNode.className = "font-medium text-slate-800";
      legNode.textContent = `${leg.trainId} | ${leg.line} | ${stationNameOnly(leg.from)} → ${stationNameOnly(leg.to)}`;
      legs.appendChild(legNode);

      const legDetail = document.createElement("div");
      legDetail.className = "text-xs font-medium text-slate-500";
      legDetail.textContent = tripPlanFormatter.buildLegDetailText(leg, (date) => formatWIBTime(date.toISOString()), stationNameOnly);
      legs.appendChild(legDetail);

      if (i < option.legs.length - 1) {
        const nextLeg = option.legs[i + 1];
        const waitMin = diffMinutes(leg.arriveAt, nextLeg.departAt);
        const waitType = tripPlanFormatter.classifyTransferWait(waitMin);
        const transfer = document.createElement("div");
        transfer.className =
          waitType === "tight transfer"
            ? "rounded-lg border border-rose-200 bg-rose-50 px-2 py-1 text-xs font-semibold text-rose-700"
            : waitType === "long wait"
              ? "rounded-lg border border-slate-200 bg-slate-50 px-2 py-1 text-xs font-semibold text-slate-700"
              : "rounded-lg border border-amber-200 bg-amber-50 px-2 py-1 text-xs font-semibold text-amber-700";
        transfer.textContent = tripPlanFormatter.buildTransferDetailText(
          leg,
          nextLeg,
          (date) => formatWIBTime(date.toISOString()),
          stationNameOnly,
        );
        legs.appendChild(transfer);

        // Alternate departure — hidden by default, revealed on demand
        const capturedLeg = leg;
        const capturedNextLeg = nextLeg;

        const altWrap = document.createElement("details");
        altWrap.className = "mt-0.5";

        const altSummary = document.createElement("summary");
        altSummary.className = "cursor-pointer list-none select-none text-xs text-slate-400 transition-colors hover:text-slate-600";
        altSummary.textContent = "↪ Missed connection?";
        altWrap.appendChild(altSummary);

        const altContent = document.createElement("div");
        altContent.className = "mt-1.5 rounded-lg border border-dashed border-slate-200 bg-slate-50 px-2.5 py-2 text-xs text-slate-500";
        altContent.textContent = "Loading…";

        const destStationID = option.legs[option.legs.length - 1].to;
        let altLoaded = false;
        altWrap.addEventListener("toggle", async () => {
          altSummary.textContent = altWrap.open ? "↩ Hide alternate" : "↪ Missed connection?";
          if (!altWrap.open || altLoaded) return;
          altLoaded = true;
          try {
            const schedules = await fetchStationSchedules(capturedNextLeg.from);
            const topology = await loadKRLTopology().catch(() => null);
            const next = await tripPlanFormatter.findAlternateDeparture(
              schedules,
              capturedNextLeg,
              destStationID,
              fetchTrainRoute,
              { topology },
            );
            if (next) {
              const depTime = formatWIBTime(next.departs_at);
              const extraWait = diffMinutes(capturedLeg.arriveAt, new Date(next.departs_at));
              altContent.innerHTML = "";
              const timeEl = document.createElement("span");
              timeEl.className = "font-semibold text-slate-700";
              timeEl.textContent = depTime;
              const sep = document.createElement("span");
              sep.className = "mx-1.5 text-slate-300";
              sep.textContent = "·";
              const trainEl = document.createElement("span");
              trainEl.textContent = `${next.train_id} | ${next.line}`;
              const badge = document.createElement("span");
              badge.className = "ml-1.5 rounded-full bg-slate-200 px-1.5 py-0.5 font-semibold text-slate-600";
              badge.textContent = `+${extraWait} min wait`;
              altContent.appendChild(timeEl);
              altContent.appendChild(sep);
              altContent.appendChild(trainEl);
              altContent.appendChild(badge);
              // Show arrival at final destination
              try {
                const altRoute = await fetchTrainRoute(next.train_id);
                const destStop = tripPlanFormatter.findArrivalStopAfterDeparture(
                  altRoute,
                  destStationID,
                  new Date(next.departs_at),
                );
                if (destStop) {
                  const arrTime = formatWIBTime(destStop.departs_at || destStop.arrives_at);
                  const sep2 = document.createElement("span");
                  sep2.className = "mx-1.5 text-slate-300";
                  sep2.textContent = "·";
                  const arrEl = document.createElement("span");
                  arrEl.className = "text-slate-500";
                  arrEl.textContent = `arr ${stationNameOnly(destStationID)} ${arrTime}`;
                  altContent.appendChild(sep2);
                  altContent.appendChild(arrEl);
                }
              } catch (_) {
                // Arrival lookup failed — still show departure info
              }
            } else {
              altContent.textContent = "No alternate train found in schedule.";
            }
          } catch (_) {
            altContent.textContent = "Could not load alternate schedule.";
          }
        });

        altWrap.appendChild(altContent);
        legs.appendChild(altWrap);
      }
    }
    card.appendChild(legs);
    planResultsNode.appendChild(card);
  }
}

function getFilteredTripOptions(options) {
  const showLongWait = Boolean(planShowLongWait && planShowLongWait.checked);
  if (showLongWait) {
    return { visible: options, hiddenLongWaitCount: 0 };
  }
  const visible = options.filter((option) => !tripPlanFormatter.optionHasLongWait(option));
  return { visible, hiddenLongWaitCount: options.length - visible.length };
}

function renderTripPlansWithStatus(options, stats) {
  const { visible, hiddenLongWaitCount } = getFilteredTripOptions(options);
  renderTripPlans(visible);

  if (visible.length === 0) {
    if (hiddenLongWaitCount > 0) {
      showPlanStatus(`No option shown. ${hiddenLongWaitCount} long-wait option(s) hidden. Enable "Show long-wait transit options" to view.`);
      return;
    }
    showPlanStatus("No route option found in current window.", true);
    return;
  }

  let message = `${visible.length} option(s) found.`;
  if (hiddenLongWaitCount > 0) {
    message += ` ${hiddenLongWaitCount} long-wait option(s) hidden.`;
  }
  if (stats?.truncated) {
    message += " Showing best bounded results.";
  }
  showPlanStatus(message);
}

async function generateTripPlan() {
  const fromID = String(planFrom.value || "").toUpperCase();
  const toID = String(planTo.value || "").toUpperCase();
  if (!fromID || !toID) {
    showPlanStatus("Please choose both origin and destination.", true);
    return;
  }
  if (fromID === toID) {
    showPlanStatus("Origin and destination cannot be the same.", true);
    return;
  }

  planButton.disabled = true;
  planResultsNode.innerHTML = "";
  showPlanStatus("Generating routes...");
  lastTripOptions = [];
  lastTripStats = null;

  try {
    const now = new Date();
    const plannerMode = plannerModeOverride();
    const payload = await fetchWithTimeout(`${API_BASE}/trip-plan`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        from_id: fromID,
        to_id: toID,
        at: now.toISOString(),
        window_minutes: TRIP_PLANNER_WINDOW_MINUTES,
        max_results: PLANNER_MAX_RESULTS,
        max_transfers: 2,
        planner_mode: plannerMode,
      }),
    });

    const options = parseTripPlanOptions(payload?.data?.options);
    const stats = payload?.data?.stats || null;
    const boundedOptions = filterTripOptionsByDepartureWindow(options, now, TRIP_PLANNER_WINDOW_MINUTES);
    lastTripOptions = boundedOptions;
    lastTripStats = stats || null;
    renderTripPlansWithStatus(boundedOptions, stats);

    const end = new Date(now.getTime() + (TRIP_PLANNER_WINDOW_MINUTES * 60 * 1000));
    const windowHint = `Window ${formatWIBTime(now.toISOString())} → ${formatWIBTime(end.toISOString())}`;
    if (planStatusNode.textContent) {
      planStatusNode.textContent = `${planStatusNode.textContent} ${windowHint}.`;
    } else {
      showPlanStatus(windowHint);
    }
    await yieldToBrowser();
  } catch (_) {
    showPlanStatus("Failed to generate route options. Please retry.", true);
  } finally {
    planButton.disabled = false;
  }
}

function getPlannerStations() {
  const useAllStations = Boolean(planShowAll && planShowAll.checked);
  if (useAllStations) {
    return stations;
  }

  const preferredSet = new Set(preferredStations);
  const filtered = stations.filter((station) => preferredSet.has(station.id));
  if (filtered.length > 0) {
    return filtered;
  }
  return stations;
}

function populateTripPlannerOptions() {
  const prefs = readTripPlannerPrefs();
  const currentFrom = String(planFrom.value || prefs.fromID || "").toUpperCase();
  const currentTo = String(planTo.value || prefs.toID || "").toUpperCase();
  const plannerStations = getPlannerStations();
  const options = plannerStations
    .map((station) => `<option value="${station.id}">${station.name} (${station.id})</option>`)
    .join("");
  planFrom.innerHTML = `<option value="">Select origin</option>${options}`;
  planTo.innerHTML = `<option value="">Select destination</option>${options}`;

  if (plannerStations.some((station) => station.id === currentFrom)) {
    planFrom.value = currentFrom;
  }
  if (plannerStations.some((station) => station.id === currentTo)) {
    planTo.value = currentTo;
  }
  if (planShowLongWait) {
    planShowLongWait.checked = Boolean(prefs.showLongWait);
  }
}

function swapTripPlannerStations() {
  const fromValue = String(planFrom.value || "").toUpperCase();
  const toValue = String(planTo.value || "").toUpperCase();
  planFrom.value = toValue;
  planTo.value = fromValue;
  saveTripPlannerPrefs({
    fromID: String(planFrom.value || "").toUpperCase(),
    toID: String(planTo.value || "").toUpperCase(),
  });
}

function renderSchedules(data) {
  cardsNode.innerHTML = "";
  windowLabel.textContent = `Window: ${data.window_start_wib} to ${data.window_end_wib}`;

  for (const station of data.stations || []) {
    const card = document.createElement("details");
    card.className = "rounded-2xl border border-slate-200 bg-white p-3 shadow-sm";

    const summary = document.createElement("summary");
    summary.className = "cursor-pointer list-none text-base font-bold tracking-tight text-slate-900";
    const schedules = Array.isArray(station.schedules) ? station.schedules : [];
    summary.textContent = `${stationDisplayName(station.station_id)} (${schedules.length})`;
    card.appendChild(summary);

    const list = document.createElement("ul");
    list.className = "mt-2 grid gap-2";

    if (schedules.length === 0) {
      const empty = document.createElement("p");
      empty.className = "mt-2 text-sm text-slate-600";
      empty.textContent = "No departures in -10 minutes to +1 hour (WIB).";
      card.appendChild(empty);
    } else {
      for (const schedule of schedules) {
        const item = document.createElement("li");
        item.className = "rounded-xl border border-slate-100 bg-slate-50 px-3 py-2";

        const time = document.createElement("div");
        time.className = "text-base font-extrabold text-cyan-700";
        time.textContent = formatWIBTime(schedule.departs_at);

        const direction = parseDirection(schedule);
        const travel = document.createElement("div");
        travel.className = "mt-0.5 text-sm font-semibold text-slate-800";
        if (direction.to) {
          travel.textContent = `To ${direction.to}${direction.from ? ` (from ${direction.from})` : ""}`;
        } else {
          travel.textContent = "Direction unavailable";
        }

        const meta = document.createElement("div");
        meta.className = "mt-0.5 text-xs text-slate-600 sm:text-sm";
        meta.textContent = `${schedule.train_id} | ${schedule.line}`;

        item.appendChild(time);
        item.appendChild(travel);
        item.appendChild(meta);
        list.appendChild(item);
      }
      card.appendChild(list);
    }

    cardsNode.appendChild(card);
  }
}

async function loadWindowSchedules() {
  if (preferredStations.length === 0) {
    showStatus("Pick at least one station.", true);
    setView("picker");
    return;
  }

  refreshButton.disabled = true;
  showStatus("Loading schedules...");

  try {
    const { windowMinutes, at } = buildWindowParams();

    const qs = new URLSearchParams({
      station_ids: preferredStations.join(","),
      window_minutes: String(windowMinutes),
      at: at.toISOString(),
    });
    const payload = await fetchWithTimeout(`${API_BASE}/schedule/window?${qs.toString()}`);
    renderSchedules(payload.data || {});
    showStatus(`Updated ${new Date().toLocaleTimeString("id-ID", { hour12: false })}`);
  } catch (_) {
    showStatus("Failed to fetch schedules. Check connection and retry.", true);
  } finally {
    refreshButton.disabled = false;
  }
}

async function init() {
  try {
    await loadStations();
  } catch (_) {
    setView("picker");
    showPickerStatus("Failed to load station list. Reload to retry.", true);
    return;
  }

  preferredStations = readPreferredStations();
  selectedSet = new Set(preferredStations);
  populateTripPlannerOptions();

  stationSearch.addEventListener("input", renderStationPicker);
  planButton.addEventListener("click", generateTripPlan);
  planShowAll.addEventListener("change", populateTripPlannerOptions);
  if (planSwap) {
    planSwap.addEventListener("click", swapTripPlannerStations);
  }
  if (planShowLongWait) {
    planShowLongWait.addEventListener("change", () => {
      saveTripPlannerPrefs({ showLongWait: Boolean(planShowLongWait.checked) });
      if (lastTripOptions.length > 0) {
        renderTripPlansWithStatus(lastTripOptions, lastTripStats);
      }
    });
  }
  planFrom.addEventListener("change", () => {
    saveTripPlannerPrefs({ fromID: String(planFrom.value || "").toUpperCase() });
  });
  planTo.addEventListener("change", () => {
    saveTripPlannerPrefs({ toID: String(planTo.value || "").toUpperCase() });
  });

  saveButton.addEventListener("click", async () => {
    savePreferredStations(Array.from(selectedSet));
    selectedSet = new Set(preferredStations);
    populateTripPlannerOptions();
    setView("schedule");
    await loadWindowSchedules();
  });

  editButton.addEventListener("click", () => {
    selectedSet = new Set(preferredStations);
    renderStationPicker();
    updateSelectionCount();
    showStatus("");
    setView("picker");
  });

  refreshButton.addEventListener("click", loadWindowSchedules);

  renderStationPicker();
  updateSelectionCount();
  if (stations.length === 0) {
    showPickerStatus("No station data available yet. Run sync, then refresh this page.", true);
  } else {
    showPickerStatus("");
  }

  if (preferredStations.length === 0) {
    setView("picker");
  } else {
    setView("schedule");
    await loadWindowSchedules();
  }
}

init();
