// Package i18n provides bilingual (EN/ID) message strings for the bot.
package i18n

import "fmt"

// Messages holds all translatable strings for a language.
type Messages struct {
	lang string
}

// New returns a Messages instance for the given language code ("en" or "id").
func New(lang string) *Messages {
	if lang != "id" {
		lang = "en"
	}
	return &Messages{lang: lang}
}

func (m *Messages) isID() bool { return m.lang == "id" }

// Welcome returns the welcome message.
func (m *Messages) Welcome(firstName string) string {
	if m.isID() {
		return fmt.Sprintf(`Halo %s! 👋 Selamat datang di KRL Commuter Bot.

Gunakan /set_route untuk mengatur stasiun asal & tujuan.
Gunakan /go_morning atau /go_evening untuk cek jadwal.
Ketik /help untuk daftar perintah lengkap.`, firstName)
	}
	return fmt.Sprintf(`Hi %s! 👋 Welcome to the KRL Commuter Bot.

Use /set_route to set your home & away stations.
Use /go_morning or /go_evening to check schedules.
Type /help for a full command list.`, firstName)
}

// Help returns the help/command list message.
func (m *Messages) Help() string {
	if m.isID() {
		return `📋 *Daftar Perintah*

*Pengaturan*
/set_route – Atur stasiun & waktu perjalanan
/set_schedule – Toggle hari kerja
/toggle_notifs – Aktifkan/matikan notifikasi harian
/settings – Lihat profil Anda

*Jadwal*
/go_morning – Jadwal pagi (rumah→kantor)
/go_evening – Jadwal sore (kantor→rumah)
/schedule – Cari jadwal manual

*Alert Satu Kali*
/schedule_once – Atur alert sekali
/list_alerts – Daftar alert terjadwal
/cancel_alert <id> – Batalkan alert

*Lainnya*
/station <nama> – Cari stasiun
/lang – Ganti bahasa (EN/ID)`
	}
	return `📋 *Command List*

*Settings*
/set_route – Set your home & away stations + times
/set_schedule – Toggle work days
/toggle_notifs – Enable/disable daily push alerts
/settings – View your profile

*Schedules*
/go_morning – Morning schedule (home→away)
/go_evening – Evening schedule (away→home)
/schedule – Manual origin/dest/time query

*One-Time Alerts*
/schedule_once – Set a one-time alert
/list_alerts – View upcoming alerts
/cancel_alert <id> – Cancel a specific alert

*Other*
/station <query> – Fuzzy station search
/lang – Toggle language (EN/ID)`
}

// AskHomeStation prompts for the home station.
func (m *Messages) AskHomeStation() string {
	if m.isID() {
		return "🏠 Masukkan nama *stasiun rumah* Anda:\n(Contoh: Depok)\n\nAtau ketik /station <nama> untuk mencari."
	}
	return "🏠 Enter your *home station* name:\n(e.g. Depok)\n\nOr type /station <name> to search."
}

// AskAwayStation prompts for the away/work station.
func (m *Messages) AskAwayStation() string {
	if m.isID() {
		return "🏢 Masukkan nama *stasiun tujuan* (kantor/tujuan) Anda:\n(Contoh: Jakarta Kota)"
	}
	return "🏢 Enter your *away station* name (work/destination):\n(e.g. Jakarta Kota)"
}

// AskMorningTime prompts for the morning departure time.
func (m *Messages) AskMorningTime() string {
	if m.isID() {
		return "⏰ Jam berapa biasanya Anda berangkat pagi?\nFormat: HH:MM (contoh: 07:00)"
	}
	return "⏰ What time do you usually leave in the morning?\nFormat: HH:MM (e.g. 07:00)"
}

// AskEveningTime prompts for the evening departure time.
func (m *Messages) AskEveningTime() string {
	if m.isID() {
		return "🌆 Jam berapa biasanya Anda pulang sore?\nFormat: HH:MM (contoh: 17:30)"
	}
	return "🌆 What time do you usually head home in the evening?\nFormat: HH:MM (e.g. 17:30)"
}

// RouteSet confirms route settings have been saved.
func (m *Messages) RouteSet(home, away, morning, evening string) string {
	if m.isID() {
		return fmt.Sprintf(`✅ *Rute tersimpan!*

🏠 Rumah: %s
🏢 Kantor: %s
🌅 Pagi: %s
🌆 Sore: %s

Gunakan /go_morning atau /go_evening untuk cek jadwal.`, home, away, morning, evening)
	}
	return fmt.Sprintf(`✅ *Route saved!*

🏠 Home: %s
🏢 Away: %s
🌅 Morning: %s
🌆 Evening: %s

Use /go_morning or /go_evening to check schedules.`, home, away, morning, evening)
}

// NoRouteSet is shown when the user hasn't configured a route yet.
func (m *Messages) NoRouteSet() string {
	if m.isID() {
		return "⚠️ Anda belum mengatur rute. Gunakan /set_route terlebih dahulu."
	}
	return "⚠️ You haven't set up a route yet. Use /set_route first."
}

// StationNotFound is shown when no station matches the query.
func (m *Messages) StationNotFound(query string) string {
	if m.isID() {
		return fmt.Sprintf("❌ Stasiun '%s' tidak ditemukan. Coba /station untuk mencari.", query)
	}
	return fmt.Sprintf("❌ Station '%s' not found. Try /station to search.", query)
}

// MultipleStationsFound lists ambiguous station matches.
func (m *Messages) MultipleStationsFound(query string, options []string) string {
	list := ""
	for i, s := range options {
		if i >= 5 {
			break
		}
		list += fmt.Sprintf("%d. %s\n", i+1, s)
	}
	if m.isID() {
		return fmt.Sprintf("🔍 Ditemukan beberapa stasiun untuk '%s':\n%s\nKetik nama stasiun yang tepat atau kode stasiun yang Anda maksud.", query, list)
	}
	return fmt.Sprintf("🔍 Multiple stations found for '%s':\n%s\nPlease type the exact station name or station code you want.", query, list)
}

// NoTrains is shown when no trains are found for the time window.
func (m *Messages) NoTrains(from, to, window string) string {
	if m.isID() {
		return fmt.Sprintf("🚫 Tidak ada kereta dari *%s* ke *%s* pada %s.", from, to, window)
	}
	return fmt.Sprintf("🚫 No trains found from *%s* to *%s* around %s.", from, to, window)
}

// TrainList formats the train schedule list header.
func (m *Messages) TrainList(from, to, window string) string {
	if m.isID() {
		return fmt.Sprintf("🚆 *Kereta %s → %s* (%s):", from, to, window)
	}
	return fmt.Sprintf("🚆 *Trains %s → %s* (%s):", from, to, window)
}

// NotificationsOn confirms notifications are enabled.
func (m *Messages) NotificationsOn() string {
	if m.isID() {
		return "🔔 Notifikasi *diaktifkan*. Anda akan mendapat info jadwal & cuaca setiap hari kerja."
	}
	return "🔔 Notifications *enabled*. You'll get daily schedule + weather alerts on work days."
}

// NotificationsOff confirms notifications are disabled.
func (m *Messages) NotificationsOff() string {
	if m.isID() {
		return "🔕 Notifikasi *dimatikan*."
	}
	return "🔕 Notifications *disabled*."
}

// Settings formats the user's current settings.
func (m *Messages) Settings(telegramID int64, home, away, morning, evening string, workDays []string, notifs bool, lang string) string {
	notifsStr := "off"
	if notifs {
		notifsStr = "on"
	}
	days := ""
	for _, d := range workDays {
		days += d + " "
	}
	if m.isID() {
		return fmt.Sprintf(`⚙️ *Pengaturan Anda*

👤 ID: %d
🏠 Rumah: %s
🏢 Kantor: %s
🌅 Waktu pagi: %s
🌆 Waktu sore: %s
📅 Hari kerja: %s
🔔 Notifikasi: %s
🌐 Bahasa: %s

Gunakan /set_route untuk mengubah rute.`, telegramID, home, away, morning, evening, days, notifsStr, lang)
	}
	return fmt.Sprintf(`⚙️ *Your Settings*

👤 ID: %d
🏠 Home: %s
🏢 Away: %s
🌅 Morning time: %s
🌆 Evening time: %s
📅 Work days: %s
🔔 Notifications: %s
🌐 Language: %s

Use /set_route to change your route.`, telegramID, home, away, morning, evening, days, notifsStr, lang)
}

// LangSwitched confirms language change.
func (m *Messages) LangSwitched() string {
	if m.isID() {
		return "🌐 Bahasa telah diganti ke *Bahasa Indonesia*."
	}
	return "🌐 Language switched to *English*."
}

// AskScheduleOrigin prompts for the origin station in /schedule.
func (m *Messages) AskScheduleOrigin() string {
	if m.isID() {
		return "🚉 Dari stasiun mana? Ketik nama stasiun asal:\n(Contoh: Rawa Buaya atau BW)\n(Ketik /station untuk mencari)"
	}
	return "🚉 From which station? Type the origin station name:\n(e.g. Rawa Buaya or BW)\n(Type /station to search)"
}

// AskScheduleDest prompts for the destination station.
func (m *Messages) AskScheduleDest() string {
	if m.isID() {
		return "🚉 Ke stasiun mana? Ketik nama stasiun tujuan:"
	}
	return "🚉 To which station? Enter the destination station name:"
}

// AskScheduleTime prompts for the time.
func (m *Messages) AskScheduleTime() string {
	if m.isID() {
		return "⏰ Jam berapa? (HH:MM atau ketik 'now' untuk sekarang)"
	}
	return "⏰ What time? (HH:MM or type 'now' for current time)"
}

// AskAlertOrigin prompts for /schedule_once origin.
func (m *Messages) AskAlertOrigin() string {
	if m.isID() {
		return "🔔 *Alert Satu Kali*\n\nDari stasiun mana? Ketik nama stasiun:"
	}
	return "🔔 *One-Time Alert*\n\nFrom which station? Enter station name:"
}

// AskAlertDest prompts for /schedule_once destination.
func (m *Messages) AskAlertDest() string {
	if m.isID() {
		return "Ke stasiun mana?"
	}
	return "To which station?"
}

// AskAlertTime prompts for /schedule_once time.
func (m *Messages) AskAlertTime() string {
	if m.isID() {
		return "Kapan? Masukkan waktu:\nFormat: HH:MM (contoh: 07:00)\n\nKetik 'tomorrow HH:MM' untuk besok, atau 'today HH:MM' untuk hari ini."
	}
	return "When? Enter the time:\nFormat: HH:MM (e.g. 07:00)\n\nType 'tomorrow HH:MM' for tomorrow, or 'today HH:MM' for today."
}

// AlertSet confirms a one-time alert has been set.
func (m *Messages) AlertSet(from, to string, t string, id string) string {
	if m.isID() {
		return fmt.Sprintf(`✅ *Alert terjadwal!*

📍 %s → %s
⏰ %s
🆔 ID: %s

/list_alerts untuk melihat daftar | /cancel_alert %s untuk membatalkan`, from, to, t, id[:8], id[:8])
	}
	return fmt.Sprintf(`✅ *Alert scheduled!*

📍 %s → %s
⏰ %s
🆔 ID: %s

/list_alerts to view all | /cancel_alert %s to cancel`, from, to, t, id[:8], id[:8])
}

// NoAlerts is shown when the user has no scheduled alerts.
func (m *Messages) NoAlerts() string {
	if m.isID() {
		return "📭 Tidak ada alert terjadwal. Gunakan /schedule_once untuk membuat."
	}
	return "📭 No scheduled alerts. Use /schedule_once to create one."
}

// AlertCancelled confirms an alert was cancelled.
func (m *Messages) AlertCancelled(id string) string {
	if m.isID() {
		return fmt.Sprintf("✅ Alert %s telah dibatalkan.", id)
	}
	return fmt.Sprintf("✅ Alert %s cancelled.", id)
}

// AlertNotFound is shown when the alert ID doesn't match.
func (m *Messages) AlertNotFound(id string) string {
	if m.isID() {
		return fmt.Sprintf("❌ Alert dengan ID '%s' tidak ditemukan.", id)
	}
	return fmt.Sprintf("❌ Alert with ID '%s' not found.", id)
}

// InvalidTime is shown for an unparseable time input.
func (m *Messages) InvalidTime() string {
	if m.isID() {
		return "❌ Format waktu tidak valid. Gunakan HH:MM (contoh: 07:00)."
	}
	return "❌ Invalid time format. Use HH:MM (e.g. 07:00)."
}

// InternalError is shown on unexpected server errors.
func (m *Messages) InternalError() string {
	if m.isID() {
		return "❌ Terjadi kesalahan. Silakan coba lagi."
	}
	return "❌ Something went wrong. Please try again."
}

// PushNotificationHeader formats the push notification header.
func (m *Messages) PushNotificationHeader(from, to string) string {
	if m.isID() {
		return fmt.Sprintf("🔔 *Notifikasi Harian*: %s → %s", from, to)
	}
	return fmt.Sprintf("🔔 *Daily Alert*: %s → %s", from, to)
}

// OneTimeAlertHeader formats the one-time alert delivery header.
func (m *Messages) OneTimeAlertHeader(from, to string) string {
	if m.isID() {
		return fmt.Sprintf("🔔 *ALERT TERJADWAL*: %s → %s", from, to)
	}
	return fmt.Sprintf("🔔 *SCHEDULED ALERT*: %s → %s", from, to)
}

// WorkSchedulePrompt returns the work days selection prompt.
func (m *Messages) WorkSchedulePrompt(current []string) string {
	days := ""
	for _, d := range current {
		days += d + " "
	}
	if m.isID() {
		return fmt.Sprintf(`📅 *Atur Hari Kerja*

Hari kerja saat ini: %s

Masukkan hari kerja baru dipisahkan koma:
Contoh: mon,tue,wed,thu,fri

Hari valid: mon tue wed thu fri sat sun`, days)
	}
	return fmt.Sprintf(`📅 *Set Work Days*

Current work days: %s

Enter new work days separated by commas:
Example: mon,tue,wed,thu,fri

Valid days: mon tue wed thu fri sat sun`, days)
}

// WorkScheduleSet confirms work days were saved.
func (m *Messages) WorkScheduleSet(days []string) string {
	d := ""
	for _, day := range days {
		d += day + " "
	}
	if m.isID() {
		return fmt.Sprintf("✅ Hari kerja tersimpan: %s", d)
	}
	return fmt.Sprintf("✅ Work days saved: %s", d)
}

func (m *Messages) MenuMorning() string {
	if m.isID() {
		return "Pagi"
	}
	return "Morning"
}

func (m *Messages) MenuEvening() string {
	if m.isID() {
		return "Sore"
	}
	return "Evening"
}

func (m *Messages) MenuPlanTrip() string {
	if m.isID() {
		return "Rencana Trip"
	}
	return "Plan Trip"
}

func (m *Messages) MenuPreferredStations() string {
	if m.isID() {
		return "Stasiun Favorit"
	}
	return "Preferred Stations"
}

func (m *Messages) MenuAlerts() string {
	if m.isID() {
		return "Alerts"
	}
	return "Alerts"
}

func (m *Messages) MenuSettings() string {
	if m.isID() {
		return "Pengaturan"
	}
	return "Settings"
}

func (m *Messages) MenuHelp() string {
	if m.isID() {
		return "Bantuan"
	}
	return "Help"
}

func (m *Messages) MenuOpenApp() string {
	if m.isID() {
		return "Buka /app"
	}
	return "Open /app"
}

func (m *Messages) MenuBack() string {
	if m.isID() {
		return "Kembali"
	}
	return "Back"
}

func (m *Messages) MenuRefresh() string {
	if m.isID() {
		return "Refresh"
	}
	return "Refresh"
}

func (m *Messages) MenuCreateAlert() string {
	if m.isID() {
		return "Buat Alert"
	}
	return "Create Alert"
}

func (m *Messages) MenuListAlerts() string {
	if m.isID() {
		return "Daftar Alert"
	}
	return "List Alerts"
}

func (m *Messages) MenuSetRoute() string {
	if m.isID() {
		return "Atur Rute"
	}
	return "Set Route"
}

func (m *Messages) MenuWorkDays() string {
	if m.isID() {
		return "Hari Kerja"
	}
	return "Work Days"
}

func (m *Messages) MenuToggleNotifications() string {
	if m.isID() {
		return "Toggle Notif"
	}
	return "Toggle Notifs"
}

func (m *Messages) MenuLanguage() string {
	if m.isID() {
		return "Bahasa"
	}
	return "Language"
}

func (m *Messages) MenuHomeToAway() string {
	if m.isID() {
		return "Rumah -> Tujuan"
	}
	return "Home -> Away"
}

func (m *Messages) MenuAwayToHome() string {
	if m.isID() {
		return "Tujuan -> Rumah"
	}
	return "Away -> Home"
}

func (m *Messages) MenuNavigationHint() string {
	if m.isID() {
		return "Pilih aksi cepat dari tombol di bawah."
	}
	return "Use the buttons below for quick actions."
}

func (m *Messages) PlanTripPrompt() string {
	if m.isID() {
		return "Pilih stasiun asal untuk trip sederhana, atau buka /app untuk planner lengkap."
	}
	return "Choose an origin for a quick trip search, or open /app for the full planner."
}

func (m *Messages) PreferredStationsPrompt() string {
	if m.isID() {
		return "Gunakan stasiun tersimpan sebagai titik awal cepat."
	}
	return "Use your saved stations as quick starting points."
}

func (m *Messages) AlertsPrompt() string {
	if m.isID() {
		return "Kelola alert satu kali dari menu ini."
	}
	return "Manage your one-time alerts from this menu."
}

func (m *Messages) SettingsPrompt() string {
	if m.isID() {
		return "Ubah rute, hari kerja, notifikasi, atau bahasa."
	}
	return "Update your route, work days, notifications, or language."
}

func (m *Messages) CommuteActionsPrompt() string {
	if m.isID() {
		return "Aksi perjalanan cepat:"
	}
	return "Quick commute actions:"
}

func (m *Messages) ButtonActionExpired() string {
	if m.isID() {
		return "Tombol ini sudah kadaluarsa. Buka menu lagi."
	}
	return "This button has expired. Open the menu again."
}

func (m *Messages) AppLinkUnavailable() string {
	if m.isID() {
		return "Link /app belum dikonfigurasi."
	}
	return "The /app link is not configured yet."
}
