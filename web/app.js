const STORAGE_KEY = "comu.preferred_stations";
const COOKIE_KEY = "preferred_stations";
const MAX_STATIONS = 5;
const WINDOW_MINUTES = 60;
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
  statusNode.classList.toggle("error", isError);
}

function showPickerStatus(message, isError = false) {
  pickerStatusNode.textContent = message;
  pickerStatusNode.classList.toggle("error", isError);
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
    stationList.innerHTML = '<p class="hint">No station data available yet. Run station sync first.</p>';
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
    stationList.innerHTML = '<p class="hint">No station matches your search.</p>';
    return;
  }

  for (const station of filtered) {
    const wrapper = document.createElement("div");
    wrapper.className = "station-item";

    const label = document.createElement("label");
    const checkbox = document.createElement("input");
    checkbox.type = "checkbox";
    checkbox.value = station.id;
    checkbox.checked = selectedSet.has(station.id);

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
    const name = document.createElement("div");
    name.className = "station-name";
    name.textContent = station.name;
    const id = document.createElement("div");
    id.className = "station-id";
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

function renderSchedules(data) {
  cardsNode.innerHTML = "";
  windowLabel.textContent = `Window: ${data.window_start_wib} to ${data.window_end_wib}`;

  for (const station of data.stations || []) {
    const card = document.createElement("article");
    card.className = "card";

    const title = document.createElement("h3");
    title.textContent = stationDisplayName(station.station_id);
    card.appendChild(title);

    const list = document.createElement("ul");
    list.className = "schedule-list";

    const schedules = Array.isArray(station.schedules) ? station.schedules : [];
    if (schedules.length === 0) {
      const empty = document.createElement("p");
      empty.className = "hint";
      empty.textContent = "No departures in +/- 1 hour (WIB).";
      card.appendChild(empty);
    } else {
      for (const schedule of schedules) {
        const item = document.createElement("li");

        const time = document.createElement("div");
        time.className = "schedule-time";
        time.textContent = formatWIBTime(schedule.departs_at);

        const meta = document.createElement("div");
        meta.className = "schedule-meta";
        meta.textContent = `${schedule.train_id} | ${schedule.line} | ${schedule.origin_id} -> ${schedule.destination_id}`;

        item.appendChild(time);
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
    const qs = new URLSearchParams({
      station_ids: preferredStations.join(","),
      window_minutes: String(WINDOW_MINUTES),
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
