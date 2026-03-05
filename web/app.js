const STORAGE_KEY = "comu.preferred_stations";
const COOKIE_KEY = "preferred_stations";
const MAX_STATIONS = 5;
const WINDOW_BEFORE_MINUTES = 10;
const WINDOW_AFTER_MINUTES = 60;
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

let stations = [];
let preferredStations = [];
let selectedSet = new Set();

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
    const windowMinutes = Math.ceil((WINDOW_BEFORE_MINUTES + WINDOW_AFTER_MINUTES) / 2);
    const atShiftMinutes = WINDOW_AFTER_MINUTES - windowMinutes;
    const at = new Date(Date.now() + atShiftMinutes * 60 * 1000);

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

  stationSearch.addEventListener("input", renderStationPicker);

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
