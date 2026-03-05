const STORAGE_KEY = "comu.preferred_stations";
const COOKIE_KEY = "preferred_stations";
const MAX_STATIONS = 5;
const WINDOW_BEFORE_MINUTES = 10;
const WINDOW_AFTER_MINUTES = 60;
const PLANNER_MAX_RUNTIME_MS = 4500;
const PLANNER_MAX_EXPANDED_STATES = 450;
const PLANNER_MAX_QUEUE_SIZE = 700;
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
const planButton = document.getElementById("plan-trip");
const planStatusNode = document.getElementById("plan-status");
const planResultsNode = document.getElementById("plan-results");

let stations = [];
let preferredStations = [];
let selectedSet = new Set();
const routeCache = new Map();
const stationScheduleCache = new Map();

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
  const payload = await fetchWithTimeout(`${API_BASE}/schedule/${encodeURIComponent(stationID)}?limit=500`);
  const schedules = Array.isArray(payload?.data) ? payload.data : [];
  stationScheduleCache.set(stationID, schedules);
  return schedules;
}

function getStopTime(stop) {
  const primary = stop.arrives_at || stop.departs_at;
  return new Date(primary);
}

function findStopIndex(route, stationID, minTime) {
  for (let i = 0; i < route.length; i++) {
    const stop = route[i];
    if (String(stop.station_id || "").toUpperCase() !== stationID) {
      continue;
    }
    if (!minTime || new Date(stop.departs_at || stop.arrives_at) >= minTime) {
      return i;
    }
  }
  return route.findIndex((stop) => String(stop.station_id || "").toUpperCase() === stationID);
}

function diffMinutes(from, to) {
  return Math.max(0, Math.round((to.getTime() - from.getTime()) / 60000));
}

function yieldToBrowser() {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

function optionKey(option) {
  return option.legs.map((leg) => `${leg.trainId}:${leg.from}->${leg.to}@${leg.departAt.toISOString()}`).join("|");
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
    legs.className = "mt-2 grid gap-1 text-sm text-slate-700";
    for (let i = 0; i < option.legs.length; i++) {
      const leg = option.legs[i];
      const legNode = document.createElement("div");
      legNode.textContent = `${leg.trainId} | ${leg.line} | ${stationNameOnly(leg.from)} → ${stationNameOnly(leg.to)}`;
      legs.appendChild(legNode);

      if (i < option.legs.length - 1) {
        const waitMin = diffMinutes(leg.arriveAt, option.legs[i + 1].departAt);
        const transfer = document.createElement("div");
        transfer.className = "text-xs font-semibold text-amber-700";
        transfer.textContent = `Transit at ${stationNameOnly(leg.to)} • wait ${waitMin} min`;
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
    const startedAt = Date.now();
    const { windowMinutes, at } = buildWindowParams();
    const qs = new URLSearchParams({
      station_ids: fromID,
      window_minutes: String(windowMinutes),
      at: at.toISOString(),
    });
    const payload = await fetchWithTimeout(`${API_BASE}/schedule/window?${qs.toString()}`);
    const schedules = payload?.data?.stations?.[0]?.schedules || [];
    const firstLegCandidates = schedules.slice(0, 20);
    if (firstLegCandidates.length === 0) {
      showPlanStatus("No departures found from selected origin in current window.", true);
      return;
    }

    const planOptions = [];
    const seen = new Set();
    const queue = [];
    const MAX_TRANSFERS = 3;
    const MIN_TRANSFER_MS = 2 * 60 * 1000;
    const MAX_TRANSFER_MS = 120 * 60 * 1000;
    const MAX_CANDIDATE_DEPARTURES = 40;
    const MAX_FORWARD_STOPS = 24;
    let expandedStates = 0;

    for (const first of firstLegCandidates) {
      const departAt = new Date(first.departs_at);
      const route = await fetchTrainRoute(first.train_id);
      if (!Array.isArray(route) || route.length === 0) {
        continue;
      }
      const fromIdx = findStopIndex(route, fromID, new Date(departAt.getTime() - MIN_TRANSFER_MS));
      if (fromIdx < 0) {
        continue;
      }
      const maxForwardStops = Math.min(route.length, fromIdx + MAX_FORWARD_STOPS);
      for (let i = fromIdx + 1; i < maxForwardStops; i++) {
        const nextStation = String(route[i].station_id || "").toUpperCase();
        if (!nextStation || nextStation === fromID) {
          continue;
        }
        const arriveAt = getStopTime(route[i]);
        const leg = {
          trainId: first.train_id,
          line: first.line,
          from: fromID,
          to: nextStation,
          departAt,
          arriveAt,
        };
        queue.push({
          stationID: nextStation,
          arriveAt,
          legs: [leg],
          visited: new Set([fromID, nextStation]),
        });
        if (queue.length >= PLANNER_MAX_QUEUE_SIZE) {
          break;
        }
      }
      if (queue.length >= PLANNER_MAX_QUEUE_SIZE) {
        break;
      }
    }

    while (queue.length > 0 && planOptions.length < PLANNER_MAX_RESULTS) {
      if (Date.now() - startedAt > PLANNER_MAX_RUNTIME_MS) {
        break;
      }
      if (expandedStates >= PLANNER_MAX_EXPANDED_STATES) {
        break;
      }
      queue.sort((a, b) => a.arriveAt - b.arriveAt);
      const current = queue.shift();
      expandedStates += 1;
      if (expandedStates % 20 === 0) {
        showPlanStatus(`Generating routes... explored ${expandedStates} states`);
        await yieldToBrowser();
      }
      const legs = current.legs;
      const lastLeg = legs[legs.length - 1];

      if (current.stationID === toID) {
        const option = {
          legs,
          departAt: legs[0].departAt,
          arriveAt: lastLeg.arriveAt,
          durationMinutes: diffMinutes(legs[0].departAt, lastLeg.arriveAt),
        };
        const key = optionKey(option);
        if (!seen.has(key)) {
          seen.add(key);
          planOptions.push(option);
        }
        continue;
      }

      if (legs.length - 1 >= MAX_TRANSFERS) {
        continue;
      }

      const departures = await fetchStationSchedules(current.stationID);
      const candidateDepartures = departures
        .filter((s) => {
          const depart = new Date(s.departs_at);
          const gap = depart.getTime() - current.arriveAt.getTime();
          return gap >= MIN_TRANSFER_MS && gap <= MAX_TRANSFER_MS;
        })
        .slice(0, MAX_CANDIDATE_DEPARTURES);

      for (const next of candidateDepartures) {
        const nextDepartAt = new Date(next.departs_at);
        const nextRoute = await fetchTrainRoute(next.train_id);
        if (!Array.isArray(nextRoute) || nextRoute.length === 0) {
          continue;
        }
        const boardIdx = findStopIndex(nextRoute, current.stationID, new Date(nextDepartAt.getTime() - MIN_TRANSFER_MS));
        if (boardIdx < 0) {
          continue;
        }

        const maxForwardStops = Math.min(nextRoute.length, boardIdx + MAX_FORWARD_STOPS);
        for (let i = boardIdx + 1; i < maxForwardStops; i++) {
          const nextStation = String(nextRoute[i].station_id || "").toUpperCase();
          if (!nextStation || current.visited.has(nextStation)) {
            continue;
          }
          const nextArriveAt = getStopTime(nextRoute[i]);
          const nextLeg = {
            trainId: next.train_id,
            line: next.line,
            from: current.stationID,
            to: nextStation,
            departAt: nextDepartAt,
            arriveAt: nextArriveAt,
          };
          const nextVisited = new Set(current.visited);
          nextVisited.add(nextStation);
          queue.push({
            stationID: nextStation,
            arriveAt: nextArriveAt,
            legs: [...legs, nextLeg],
            visited: nextVisited,
          });
          if (queue.length >= PLANNER_MAX_QUEUE_SIZE) {
            break;
          }
        }
        if (queue.length >= PLANNER_MAX_QUEUE_SIZE) {
          break;
        }
      }
    }

    planOptions.sort((a, b) => a.departAt - b.departAt || a.arriveAt - b.arriveAt);
    renderTripPlans(planOptions.slice(0, PLANNER_MAX_RESULTS));
    if (Date.now() - startedAt > PLANNER_MAX_RUNTIME_MS || expandedStates >= PLANNER_MAX_EXPANDED_STATES) {
      showPlanStatus(`Showing best ${Math.min(planOptions.length, PLANNER_MAX_RESULTS)} result(s) from bounded search.`);
    }
  } catch (_) {
    showPlanStatus("Failed to generate route options. Please retry.", true);
  } finally {
    planButton.disabled = false;
  }
}

function populateTripPlannerOptions() {
  const options = stations
    .map((station) => `<option value="${station.id}">${station.name} (${station.id})</option>`)
    .join("");
  planFrom.innerHTML = `<option value="">Select origin</option>${options}`;
  planTo.innerHTML = `<option value="">Select destination</option>${options}`;
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

  saveButton.addEventListener("click", async () => {
    savePreferredStations(Array.from(selectedSet));
    selectedSet = new Set(preferredStations);
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
