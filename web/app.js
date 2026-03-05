const STORAGE_KEY = "comu.preferred_stations";
const COOKIE_KEY = "preferred_stations";
const MAX_STATIONS = 5;
const WINDOW_BEFORE_MINUTES = 10;
const WINDOW_AFTER_MINUTES = 60;
const PLANNER_MAX_RESULTS = 8;
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
const planShowAll = document.getElementById("plan-show-all");
const planButton = document.getElementById("plan-trip");
const planStatusNode = document.getElementById("plan-status");
const planResultsNode = document.getElementById("plan-results");

let stations = [];
let preferredStations = [];
let selectedSet = new Set();
const routeCache = new Map();
const stationScheduleCache = new Map();
const tripPlanFormatter = window.TripPlanFormatter || {
  buildLegDetailText: (leg, formatTime, stationNameOnlyFn) =>
    `${stationNameOnlyFn(leg.from)} dep ${formatTime(leg.departAt)} • ${stationNameOnlyFn(leg.to)} arr ${formatTime(leg.arriveAt)}`,
  buildTransferDetailText: (firstLeg, secondLeg, formatTime, stationNameOnlyFn) => {
    const waitMin = Math.max(0, Math.round((secondLeg.departAt.getTime() - firstLeg.arriveAt.getTime()) / 60000));
    return `Transit at ${stationNameOnlyFn(firstLeg.to)} • arrive ${formatTime(firstLeg.arriveAt)} • depart ${formatTime(secondLeg.departAt)} • wait ${waitMin} min`;
  },
  classifyTransferWait: () => "",
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

async function fetchWithTimeout(url, timeoutMs = 10000) {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetch(url, {
      signal: controller.signal,
      headers: { Accept: "application/json" },
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

function renderTripPlans(options) {
  planResultsNode.innerHTML = "";

  if (options.length === 0) {
    showPlanStatus("No route option found in current window.", true);
    return;
  }

  showPlanStatus(`${options.length} option(s) found.`);
  for (const option of options) {
    const card = document.createElement("article");
    card.className = "rounded-xl border border-slate-200 bg-white p-3";

    const head = document.createElement("div");
    head.className = "flex items-center justify-between gap-2";
    const mode = document.createElement("span");
    const transitCount = Math.max(0, option.legs.length - 1);
    mode.className =
      transitCount === 0
        ? "rounded-full bg-emerald-100 px-2 py-1 text-xs font-bold text-emerald-700"
        : "rounded-full bg-amber-100 px-2 py-1 text-xs font-bold text-amber-700";
    mode.textContent = transitCount === 0 ? "Direct" : `${transitCount} Transit`;
    const timing = document.createElement("div");
    timing.className = "text-sm font-bold text-slate-900";
    timing.textContent = `${formatWIBTime(option.departAt.toISOString())} → ${formatWIBTime(option.arriveAt.toISOString())}`;
    head.appendChild(mode);
    head.appendChild(timing);

    const meta = document.createElement("div");
    meta.className = "mt-1 text-xs text-slate-600";
    meta.textContent = `Duration ${option.durationMinutes} min`;

    card.appendChild(head);
    card.appendChild(meta);

    const legs = document.createElement("div");
    legs.className = "mt-2 grid gap-2 text-sm text-slate-700";
    for (let i = 0; i < option.legs.length; i++) {
      const leg = option.legs[i];
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
      }
    }
    card.appendChild(legs);

    planResultsNode.appendChild(card);
  }
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

  try {
    const originSchedules = await fetchStationSchedules(fromID);
    if (!Array.isArray(originSchedules) || originSchedules.length === 0) {
      showPlanStatus("No departures found from selected origin in current window.", true);
      return;
    }

    const planner = window.PlannerCore;
    if (!planner || typeof planner.findTripOptions !== "function") {
      throw new Error("PlannerCore unavailable");
    }

    const { options, stats } = await planner.findTripOptions({
      fromID,
      toID,
      now: new Date(),
      firstLegSchedules: originSchedules,
      getRoute: fetchTrainRoute,
      getStationSchedules: fetchStationSchedules,
      config: {
        maxResults: PLANNER_MAX_RESULTS,
      },
    });

    renderTripPlans(options);
    if (stats?.truncated) {
      showPlanStatus(`Showing best ${options.length} result(s) from bounded search.`);
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
  const currentFrom = String(planFrom.value || "").toUpperCase();
  const currentTo = String(planTo.value || "").toUpperCase();
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
