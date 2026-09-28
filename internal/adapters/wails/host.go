package wails

import (
	"database/sql"
	"github.com/nord-launcher/launcher/internal/core/auth"
	"github.com/nord-launcher/launcher/internal/core/content"
	"github.com/nord-launcher/launcher/internal/core/content/curseforge"
	"github.com/nord-launcher/launcher/internal/core/content/modrinth"
	"github.com/nord-launcher/launcher/internal/core/java"
	"github.com/nord-launcher/launcher/internal/core/launch"
	"github.com/nord-launcher/launcher/internal/core/loadermeta"
	"github.com/nord-launcher/launcher/internal/core/ports"
	"github.com/nord-launcher/launcher/internal/core/storage"
	"github.com/nord-launcher/launcher/internal/core/updater"
	"net/http"
)

// Host is the embedding application's wiring facade. These setters used to be
// public methods on *WailsAdapter - which Wails *binds*: the beta.20 binding
// generator walks every exported method of the bound struct
// (pkg/application/bindings.go, ptrType.NumMethod loop) and offers it to the
// webview runtime; there is no //wails:ignore directive in this version. A
// compromised webview (XSS via user-authored Modrinth descriptions is a live
// vector) must not be able to call SetGameManifestURL and repoint the Mojang
// version manifest (-> attacker-controlled JARs), swap the HTTP client, or
// rebind the database. The methods are therefore unexported on the adapter,
// and the host reaches them through this type, which is never bound.
//
// The registry test reverse-checks *WailsAdapter by reflection: any exported
// method outside the declared webview registry fails CI.
type Host struct{ a *WailsAdapter }

// NewHost returns the wiring facade for a constructed adapter. Wire everything
// before handing the adapter to the application.
func NewHost(a *WailsAdapter) *Host { return &Host{a: a} }

func (h *Host) SetAllowedHosts(hosts []string) {
	h.a.setAllowedHosts(hosts)
}

func (h *Host) SetAuth(authSvc *auth.AuthService, accountRepo ports.AccountRepository) {
	h.a.setAuth(authSvc, accountRepo)
}

func (h *Host) SetContent(mr *modrinth.Client, cf *curseforge.Client) {
	h.a.setContent(mr, cf)
}

func (h *Host) SetCurseForgePackImporter(imp *launch.CurseForgePackImporter) {
	h.a.setCurseForgePackImporter(imp)
}

func (h *Host) SetDB(db *sql.DB) {
	h.a.setDB(db)
}

func (h *Host) SetFilePicker(fn func() (string, error)) {
	h.a.setFilePicker(fn)
}

func (h *Host) SetFileSystem(fs ports.FileSystem, instancesDir string) {
	h.a.setFileSystem(fs, instancesDir)
}

func (h *Host) SetGameManifestURL(u string) {
	h.a.setGameManifestURL(u)
}

func (h *Host) SetHTTPClient(client *http.Client) {
	h.a.setHTTPClient(client)
}

func (h *Host) SetImporter(imp *launch.InstanceImporter) {
	h.a.setImporter(imp)
}

func (h *Host) SetInstalledModsRepo(repo *storage.InstalledModsRepository) {
	h.a.setInstalledModsRepo(repo)
}

func (h *Host) SetIntegrityVerifier(v ports.IntegrityVerifier) {
	h.a.setIntegrityVerifier(v)
}

func (h *Host) SetJavaDetector(jd ports.JavaDetector) {
	h.a.setJavaDetector(jd)
}

func (h *Host) SetJavaManager(jm *java.JavaManager) {
	h.a.setJavaManager(jm)
}

func (h *Host) SetLoaderResolver(r *loadermeta.Resolver) {
	h.a.setLoaderResolver(r)
}

func (h *Host) SetMrPackExporter(exporter *content.MrPackExporter) {
	h.a.setMrPackExporter(exporter)
}

func (h *Host) SetMrPackImporter(importer *content.MrPackImporter) {
	h.a.setMrPackImporter(importer)
}

func (h *Host) SetOnLogBatch(fn func(instanceID string, lines []string)) {
	h.a.setOnLogBatch(fn)
}

func (h *Host) SetRelauncher(fn updater.RelauncherFunc) {
	h.a.setRelauncher(fn)
}

func (h *Host) SetSettings(repo *storage.SettingsRepository) {
	h.a.setSettings(repo)
}

func (h *Host) SetUpdater(u *updater.AutoUpdater) {
	h.a.setUpdater(u)
}

func (h *Host) SetVersion(v string) {
	h.a.setVersion(v)
}

// RecordCrash is the host-internal bridge to the adapter's crash hook (called
// from the InstanceService onCrash callback). Unexported on the bound adapter
// on purpose; this facade is never bound to the webview.
func (h *Host) RecordCrash(instanceID string, report *launch.CrashReport) {
	h.a.recordCrash(instanceID, report)
}
