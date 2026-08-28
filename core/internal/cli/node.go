package cli

// Node commands: the machine the panel is installed on, and the agent that
// makes it manageable.
//
// # What this file is for, and what it is deliberately not
//
// VKAI Panel was built as a control plane for a fleet: the panel here, agents on
// other machines. That is right for a hosting provider with racks, and it left
// one gap - after "install the panel" the panel managed nothing at all, not even
// the machine it had just been installed on, so a first website needed a second
// server.
//
// Closing that gap has two halves, and only the second one lives here.
//
// The first half - deciding which servers row is this machine, measuring the
// machine, and writing the row idempotently - belongs to the panel itself:
// internal/localnode identifies the host, repository.RegisterLocalNode writes
// the row, and ServerService.RegisterLocalNode ties the two together and is
// called at every API start. This file calls that same path rather than
// carrying a second implementation of it, because two pieces of code deciding
// independently "which row is this machine" is exactly how one machine becomes
// two rows.
//
// The second half is the agent, and it is what this file adds: the node row is
// only a record until a certificate-holding agent is running on the machine it
// describes. "vkai node register" enables vkai-agent, mints the single-use token
// it enrols with, waits for the certificate to be issued, and then removes the
// spent token. Nothing else in the product does that; before it, an operator had
// to enable the unit and paste a token by hand on the machine they had just
// installed on.
//
// # Why minting a token locally is safe HERE and would not be elsewhere
//
// Enrolment needs a single-use token because the token is the only thing that
// authenticates a joining agent before it has a certificate. Across a network
// that token has to be carried by a human, over a channel the panel does not
// control, to a machine the panel cannot yet identify. Every risk in that
// sentence is about the carrying: a bearer secret in transit, on a screen, in a
// clipboard, in shell history.
//
// Here there is no carrying. The panel and the agent are the same machine. The
// token is requested over the loopback interface, written to a file only root
// can read, and spent by a process on that same machine seconds later; it never
// reaches a network interface and never reaches a terminal. The only party who
// could read it is root on this host, who already holds the CA private key, the
// database password and the panel's TLS key - so the token grants nothing that
// was not already held, and it is deleted from the file as soon as it is spent.
//
// None of that survives a move to a second machine: the moment the token has to
// reach another host it is a bearer secret in transit again, and it goes back
// through the operator - "Servers -> Add agent", paste, restart the agent. That
// path is unchanged, and "vkai node register --enrolment-token" is how this
// command consumes it when an operator would rather mint it themselves.

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/agentpki"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/auth"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/localnode"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/service"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	// agentService is the unit that has to be running for this machine to be
	// managed rather than merely listed.
	agentService = "vkai-agent"

	// enrolmentTTL is how long the minted token stays usable. On this path it is
	// spent within seconds; the window exists only so that a slow first start of
	// the agent does not have to be driven again by hand.
	enrolmentTTL = 30 * time.Minute

	// exitNotEnrolled is returned when the node row is in place but its agent is
	// not enrolled. The two are separated because they fail for different
	// reasons and are repaired by different commands, and because the row is the
	// half an operator cannot safely reproduce by hand.
	exitNotEnrolled = 3
)

var (
	nodePanelURL     string
	nodeEnrolmentTok string
	nodeTimeout      time.Duration
	nodeSkipAgent    bool
	nodeRebind       bool
)

var nodeCmd = &cobra.Command{
	Use:   "node",
	Short: "The machines this panel manages",
	Long: `Commands for the managed nodes, starting with the one the panel is
installed on. "vkai node register" is what makes this machine usable; the
installer runs it, and it is safe to run again at any time.`,
}

var nodeRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "Register this machine as a managed node and enrol its agent",
	Long: `Registers the machine this panel runs on as a managed node, then
enables the local agent and enrols it in the panel's certificate authority.

Registration is idempotent. The node is keyed on the node id in
<EtcRoot>/node.json - not on the hostname, which changes under a running
installation - so a machine that has been renamed keeps the row it already had,
and running this twice produces one row. Nothing an operator has edited on the
row is overwritten; only the facts read from the machine itself are refreshed.

Exit status 0 means the node is registered and its agent is enrolled. Exit
status 3 means the row is in place but the agent is not enrolled yet, and the
command printed what to run next.`,
	Run: runNodeRegister,
}

var nodeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the machines this panel manages",
	Run:   runNodeList,
}

func init() {
	nodeCmd.AddCommand(nodeRegisterCmd)
	nodeCmd.AddCommand(nodeListCmd)

	nodeRegisterCmd.Flags().StringVar(&nodePanelURL, "panel-url", "",
		"Base URL the local agent uses to reach this panel (default: derived from the panel configuration)")
	nodeRegisterCmd.Flags().StringVar(&nodeEnrolmentTok, "enrolment-token", "",
		"Use this enrolment token instead of minting one (paste from Servers -> Add agent)")
	nodeRegisterCmd.Flags().DurationVar(&nodeTimeout, "timeout", 120*time.Second,
		"How long to wait for the panel API, and then for the agent to enrol")
	nodeRegisterCmd.Flags().BoolVar(&nodeSkipAgent, "skip-agent", false,
		"Write the node row only: do not touch the agent service")
	nodeRegisterCmd.Flags().BoolVar(&nodeRebind, "rebind", false,
		"Re-record the machine witness for an identity that no longer matches this machine. "+
			"Only for a panel configuration restored onto rebuilt hardware")
}

// ============================================================
// REGISTER
// ============================================================

func runNodeRegister(cmd *cobra.Command, args []string) {
	if os.Geteuid() != 0 {
		printError("'vkai node register' needs root: it reads the panel secrets and drives systemd. Run: sudo vkai node register")
	}
	loadPanelEnv()

	db, err := openPanelDB()
	if err != nil {
		printError("Cannot open the panel database: %v", err)
	}
	defer db.Close()

	// Asked before the service is built, because building it registers: the
	// panel's own startup path runs from the constructor, so by the time
	// RegisterLocalNode is called below the row already exists and the honest
	// answer to "what did this command do" would be lost.
	before := describeExistingNodes(db)

	// The service owns the identity, the measurement and the row. This process
	// only asks it to do the work and reports what it did.
	serverService := service.NewServerService(repository.NewServerRepository(db), zap.NewNop())
	defer serverService.StopLocalNodeHeartbeat()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := serverService.RegisterLocalNode(ctx, service.LocalNodeOptions{Rebind: nodeRebind})
	if err != nil {
		reportRegistrationFailure(err)
	}
	reportRegistrationResult(result, before)

	// The panel API runs as its own account and reads this file on every start.
	// When registration was driven from a root shell before the API ever ran,
	// the file would otherwise be root-owned and 0600, and the panel would
	// report that it has no local node while the row sits in the database.
	alignIdentityOwnership()

	if nodeSkipAgent {
		printInfo("--skip-agent: the agent was not touched. Enrol it with: sudo vkai node register")
		return
	}
	if !enrolAgent(result.NodeID.String(), result.Server.Hostname) {
		os.Exit(exitNotEnrolled)
	}
}

func reportRegistrationFailure(err error) {
	switch {
	case errors.Is(err, repository.ErrLocalNodeBookkeepingUnavailable):
		printError("This database has no server_local_node table, so there is nowhere to record that this "+
			"machine is a managed node.\n  Apply it and try again:\n"+
			"    psql -d %s -f %s/migrations/pending/local_node.sql\n    sudo vkai node register",
			panelEnv("VKAI_DB_NAME", "vkai_panel"), config.CoreRoot())
	case errors.Is(err, localnode.ErrIdentityMismatch):
		printError("The node identity in %s does not describe this machine.\n"+
			"  If this panel's configuration was restored onto rebuilt hardware, re-run with --rebind.\n"+
			"  If it was not, this database belongs to a different machine and must not be adopted.\n  %v",
			localnode.IdentityPath(config.EtcRoot()), err)
	default:
		printError("Cannot register this machine as a managed node: %v", err)
	}
}

// priorNodes is the state of the world before this command touched it: which
// nodes the database already marked as a panel host, and whether this machine
// held an identity file naming one of them. It is what makes the sentence
// printed afterwards true rather than merely likely.
type priorNodes struct {
	ids         map[string]bool
	hadIdentity bool
}

func (p priorNodes) knew(id string) bool { return p.ids[id] }

// describeExistingNodes is deliberately forgiving. A database without the marker
// table, an unreadable identity file and a query that fails all answer "nothing
// was known", which understates rather than overstates what was there.
func describeExistingNodes(db *sqlx.DB) priorNodes {
	prior := priorNodes{ids: map[string]bool{}}
	if _, err := localnode.LoadIdentity(config.EtcRoot()); err == nil {
		prior.hadIdentity = true
	}
	rows, err := db.Query(`
		SELECT l.server_id::text
		  FROM server_local_node l
		  JOIN servers s ON s.id = l.server_id
		 WHERE s.deleted_at IS NULL`)
	if err != nil {
		return prior
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			prior.ids[id] = true
		}
	}
	return prior
}

func reportRegistrationResult(result *service.LocalNodeRegistrationResult, before priorNodes) {
	switch {
	case !before.knew(result.NodeID.String()):
		printSuccess("This machine is now this panel's first managed node.")
	case !before.hadIdentity:
		printSuccess("This machine had no identity file, but the database already held the node it registered " +
			"before; that node was adopted rather than a second one created.")
	case result.Rebound:
		printSuccess("The node identity was re-bound to this machine.")
	default:
		printInfo("This machine is already registered. Its facts were refreshed; nothing an operator set was changed.")
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "  Node ID:\t%s\n", result.NodeID)
	fmt.Fprintf(w, "  Hostname:\t%s\n", result.Server.Hostname)
	fmt.Fprintf(w, "  Address:\t%s\n", orDash(result.Server.IPAddress))
	if result.Server.IPv6Address != "" {
		fmt.Fprintf(w, "  IPv6:\t%s\n", result.Server.IPv6Address)
	}
	fmt.Fprintf(w, "  OS / kernel:\t%s / %s\n", orDash(result.Server.OS), orDash(result.Server.Kernel))
	fmt.Fprintf(w, "  CPU / RAM / disk:\t%d cores / %s / %s\n",
		result.Server.CPUCores, humanBytes(result.Server.RAMTotal), humanBytes(result.Server.DiskTotal))
	fmt.Fprintf(w, "  Role / status:\t%s / %s\n", orDash(result.Server.Role), orDash(result.Server.Status))
	fmt.Fprintf(w, "  Identity file:\t%s\n", localnode.IdentityPath(config.EtcRoot()))
	if result.WitnessSource != "" {
		fmt.Fprintf(w, "  Machine witness:\t%s (%s)\n", result.WitnessSource, verifiedWord(result.Verified))
	} else {
		fmt.Fprintf(w, "  Machine witness:\tnone available on this host - the node id alone stands for it\n")
	}
	w.Flush()
}

func verifiedWord(verified bool) string {
	if verified {
		return "agrees with this machine"
	}
	return "not confirmed"
}

// alignIdentityOwnership makes node.json readable by whoever owns the panel's
// .env, which is the account vkai-api runs as. The panel's own copy of the
// identity is the one that decides whether an operation may take the local
// route, so a file only root can read is a node the panel cannot see.
func alignIdentityOwnership() {
	path := localnode.IdentityPath(config.EtcRoot())
	var env syscall.Stat_t
	if err := syscall.Stat(config.EnvFile(), &env); err != nil {
		return
	}
	var identity syscall.Stat_t
	if err := syscall.Stat(path, &identity); err != nil {
		return
	}
	if identity.Uid == env.Uid && identity.Gid == env.Gid {
		return
	}
	if err := os.Chown(path, int(env.Uid), int(env.Gid)); err != nil {
		printWarn("Cannot give %s the same owner as %s (%v). The panel process may not be able to read it.", path, config.EnvFile(), err)
	}
}

// ============================================================
// THE AGENT ON THIS MACHINE
// ============================================================

// enrolAgent gets the agent on this machine to the point of holding a
// certificate issued by this panel, and reports whether it got there.
//
// Every failure prints the one command that resumes the work. An installer that
// stops dead because a service did not come up is worse than one that finishes,
// leaves a working panel, and says exactly what is missing.
func enrolAgent(nodeID, hostname string) bool {
	if err := ensureAgentStateDir(); err != nil {
		printWarn("Cannot prepare %s: %v", agentStateDir(), err)
		printWarn("Retry with: sudo vkai node register")
		return false
	}

	panelURL := resolvePanelURL()

	if rec := enrolledAgentFor(nodeID); rec != nil {
		printInfo("The agent on this machine is already enrolled (agent_id %s, certificate valid until %s).",
			rec.AgentID, rec.Current.NotAfter.Local().Format("2006-01-02 15:04"))
		// Rewritten even though nothing needs enrolling: it refreshes the panel
		// URL after an entrance or port change, and it clears a token that was
		// left behind by a run that minted one and then could not finish.
		if err := writeAgentEnv(panelURL, ""); err != nil {
			printWarn("Cannot refresh %s: %v", agentEnvFile(), err)
		}
		if err := startAgent(); err != nil {
			printWarn("The agent is enrolled but its service did not start: %v", err)
			printWarn("Inspect it with: journalctl -u %s -n 80", agentService)
			return false
		}
		printSuccess("The agent is enabled at boot and running.")
		return true
	}

	token := strings.TrimSpace(nodeEnrolmentTok)
	if token == "" {
		if err := waitForAPI(nodeTimeout); err != nil {
			printWarn("The panel API did not answer: %v", err)
			printWarn("The node is registered but its agent is not enrolled.")
			printWarn("Start the panel and retry with: sudo vkai node register")
			return false
		}
		minted, err := mintEnrolmentToken(panelURL, nodeID, hostname)
		if err != nil {
			printWarn("Could not mint an enrolment token: %v", err)
			printWarn("The node is registered but its agent is not enrolled.")
			printWarn("Retry with:  sudo vkai node register")
			printWarn("Or mint one in the panel (Servers -> Add agent) and run:")
			printWarn("  sudo vkai node register --enrolment-token <token>")
			return false
		}
		token = minted
	}

	if err := writeAgentEnv(panelURL, token); err != nil {
		printWarn("Cannot write %s: %v", agentEnvFile(), err)
		return false
	}
	if err := startAgent(); err != nil {
		printWarn("The agent service did not start: %v", err)
		printWarn("Inspect it with: journalctl -u %s -n 80", agentService)
		printWarn("Then retry with: sudo vkai node register")
		return false
	}

	rec := waitForEnrolment(nodeID, nodeTimeout)
	if rec == nil {
		printWarn("The agent started but has not enrolled within %s.", nodeTimeout)
		printWarn("Inspect it with: journalctl -u %s -n 80", agentService)
		printWarn("Then retry with: sudo vkai node register")
		return false
	}

	// The token is spent. Leaving a dead secret in a file teaches the wrong
	// habit and makes the next reader wonder whether it still works.
	if err := writeAgentEnv(panelURL, ""); err != nil {
		printWarn("The agent enrolled, but the spent token could not be cleared from %s: %v", agentEnvFile(), err)
	}
	printSuccess("The agent on this machine is enrolled (agent_id %s).", rec.AgentID)
	printInfo("This machine is under management. Create a website, a database or a certificate on it now.")
	printInfo("To add ANOTHER machine later: Servers -> Add agent in the panel, then start vkai-agent there with the token it shows.")
	return true
}

func agentEnvFile() string { return filepath.Join(config.EtcRoot(), "agent.env") }

func agentStateDir() string {
	if v := strings.TrimSpace(os.Getenv("VKAI_AGENT_STATE_DIR")); v != "" {
		return v
	}
	return filepath.Join(config.SSLRoot(), "agent")
}

// ensureAgentStateDir creates the directory the agent writes its key and
// certificate into. It has to exist before the unit starts: vkai-agent.service
// lists it under ReadWritePaths, and systemd cannot bind a writable mount over a
// path that is not there. Mode 0700 and root-owned: the panel account has no
// business reading the private key of the thing it authenticates.
func ensureAgentStateDir() error {
	return os.MkdirAll(agentStateDir(), 0o700)
}

// writeAgentEnv writes the environment file vkai-agent.service reads. It is kept
// apart from the panel's .env on purpose: the installer rewrites .env wholesale
// on every run, and what makes this machine a node has to survive that.
func writeAgentEnv(panelURL, enrolmentToken string) error {
	var buf bytes.Buffer
	buf.WriteString("# VKAI Panel - the agent on this machine.\n")
	buf.WriteString("# Written by \"vkai node register\". vkai-agent.service reads it AFTER\n")
	buf.WriteString("# " + config.EnvFile() + ", so the values here win.\n")
	buf.WriteString("#\n")
	buf.WriteString("# An enrolment token appears here only between being minted and being spent.\n")
	buf.WriteString("# It is single use: once the agent holds a certificate the line is removed,\n")
	buf.WriteString("# and the value would no longer authenticate anything in any case.\n")
	fmt.Fprintf(&buf, "VKAI_PANEL_URL=%s\n", panelURL)
	fmt.Fprintf(&buf, "VKAI_AGENT_STATE_DIR=%s\n", agentStateDir())
	if caFile := panelCAFile(panelURL); caFile != "" {
		fmt.Fprintf(&buf, "VKAI_PANEL_CA_FILE=%s\n", caFile)
	}
	if enrolmentToken != "" {
		fmt.Fprintf(&buf, "VKAI_AGENT_ENROLMENT_TOKEN=%s\n", enrolmentToken)
	}

	if err := os.MkdirAll(filepath.Dir(agentEnvFile()), 0o700); err != nil {
		return err
	}
	tmp := agentEnvFile() + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, agentEnvFile())
}

// panelCAFile is the certificate the agent has to trust to reach the panel over
// TLS. On this machine that is the panel's own certificate, which on a default
// install is self-signed and therefore in no system trust store. Reading it from
// the local filesystem is a stronger anchor than any store, and verification is
// never switched off.
func panelCAFile(panelURL string) string {
	if !strings.HasPrefix(panelURL, "https://") {
		return ""
	}
	cert := panelEnv("VKAI_PANEL_TLS_CERT", filepath.Join(config.SSLRoot(), "panel.crt"))
	if _, err := os.Stat(cert); err != nil {
		return ""
	}
	return cert
}

// startAgent enables the unit at boot and starts it now. "enable" is what turns
// the agent from something an operator has to remember into part of the
// installation.
func startAgent() error {
	if err := runSystemctl("enable", agentService); err != nil {
		return err
	}
	return runSystemctl("restart", agentService)
}

func runSystemctl(args ...string) error {
	output, err := exec.Command("systemctl", args...).CombinedOutput()
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(string(output))
	if message == "" {
		return err
	}
	return fmt.Errorf("systemctl %s: %v: %s", strings.Join(args, " "), err, message)
}

// enrolledAgentFor answers the only question that matters about the channel:
// does the panel's certificate authority hold a live agent identity for this
// node? Its state file is the authority on that. It is read here and never
// written, so a running panel - which holds the same state in memory - is not
// disturbed.
func enrolledAgentFor(nodeID string) *agentpki.AgentRecord {
	path := filepath.Join(agentpki.DefaultDir(), "state.json")
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	store, err := agentpki.NewFileStore(path)
	if err != nil {
		return nil
	}
	records, err := store.ListAgents(context.Background())
	if err != nil {
		return nil
	}
	for _, rec := range records {
		if rec == nil || rec.Revoked {
			continue
		}
		if rec.Role == agentpki.RoleAgent && rec.ServerID == nodeID {
			return rec
		}
	}
	return nil
}

func waitForEnrolment(nodeID string, timeout time.Duration) *agentpki.AgentRecord {
	deadline := time.Now().Add(timeout)
	for {
		if rec := enrolledAgentFor(nodeID); rec != nil {
			return rec
		}
		if time.Now().After(deadline) {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
}

// ============================================================
// TALKING TO THE PANEL API OVER LOOPBACK
// ============================================================

// resolvePanelURL builds the base URL the local agent uses. Two rules decide it,
// and both come from the access gate that wraps the whole API:
//
//   - The security entrance is part of the path. A request that does not carry
//     it gets a neutral 404, agent or not.
//   - When VKAI_PANEL_DOMAIN is set the gate pins the Host header to that name,
//     so a request addressed to 127.0.0.1 is refused. The agent then has to use
//     the same URL a human does, and trust the panel's certificate.
func resolvePanelURL() string {
	if v := strings.TrimSpace(nodePanelURL); v != "" {
		return strings.TrimRight(v, "/")
	}
	entrance := ""
	if !isFalse(panelEnv("VKAI_PANEL_ENTRANCE_ENABLED", "true")) {
		entrance = normalizeEntrancePath(panelEnv("VKAI_PANEL_ENTRANCE", ""))
	}
	if domain := strings.TrimSpace(panelEnv("VKAI_PANEL_DOMAIN", "")); domain != "" {
		return fmt.Sprintf("%s://%s:%s%s",
			panelEnv("VKAI_PANEL_PUBLIC_SCHEME", "https"),
			domain,
			panelEnv("VKAI_PANEL_PUBLIC_PORT", "8888"),
			entrance)
	}
	return fmt.Sprintf("http://127.0.0.1:%s%s", apiBindPort(), entrance)
}

func apiBindPort() string {
	for _, key := range []string{"VKAI_PANEL_PORT", "VKAI_SERVER_PORT"} {
		if v := strings.TrimSpace(panelEnv(key, "")); v != "" {
			return v
		}
	}
	return "30110"
}

func normalizeEntrancePath(raw string) string {
	value := "/" + strings.Trim(strings.TrimSpace(raw), "/")
	if value == "/" {
		return ""
	}
	return value
}

// waitForAPI blocks until /health answers. That path is answered before the
// access gate applies any rule, so it needs neither the security entrance nor a
// place on the IP allow list, which makes it the one honest way to ask whether
// the panel is up.
func waitForAPI(timeout time.Duration) error {
	url := fmt.Sprintf("http://127.0.0.1:%s/health", apiBindPort())
	client := &http.Client{Timeout: 3 * time.Second}
	deadline := time.Now().Add(timeout)
	var last error
	for {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 500 {
				return nil
			}
			last = fmt.Errorf("%s answered %s", url, resp.Status)
		} else {
			last = err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s did not answer within %s: %v", url, timeout, last)
		}
		time.Sleep(2 * time.Second)
	}
}

type mintResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Error   string `json:"error"`
	Data    struct {
		Token       string `json:"token"`
		EnrolmentID string `json:"enrolment_id"`
	} `json:"data"`
}

// mintEnrolmentToken asks the running panel for a single-use token, through the
// same endpoint the "Add agent" button uses.
//
// It goes through the API rather than writing the certificate authority's state
// directly because the API process owns that state: it holds it in memory and
// rewrites the file on every change, so a token written underneath it would be
// invisible to it and then overwritten by the next thing it wrote. That is also
// why this step runs after the services are up.
func mintEnrolmentToken(panelURL, nodeID, hostname string) (string, error) {
	bearer, err := localAdminToken()
	if err != nil {
		return "", err
	}

	body, err := json.Marshal(map[string]interface{}{
		"server_id":   nodeID,
		"hostname":    hostname,
		"ttl_seconds": int(enrolmentTTL.Seconds()),
	})
	if err != nil {
		return "", err
	}

	endpoint := panelURL + "/api/v1/agent-pki/enrolments"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearer)

	resp, err := panelHTTPClient(panelURL).Do(req)
	if err != nil {
		return "", fmt.Errorf("POST %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var parsed mintResponse
	_ = json.Unmarshal(raw, &parsed)

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return "", fmt.Errorf("the panel answered 404 for %s. The security entrance in that URL is "+
			"probably not the current one: check 'vkai entrance', or pass --panel-url", endpoint)
	case resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK:
		detail := strings.TrimSpace(parsed.Error + parsed.Message)
		if detail == "" {
			detail = strings.TrimSpace(string(raw))
		}
		return "", fmt.Errorf("the panel answered %s: %s", resp.Status, truncateText(detail, 300))
	case strings.TrimSpace(parsed.Data.Token) == "":
		return "", errors.New("the panel accepted the request but returned no token")
	}
	return parsed.Data.Token, nil
}

// panelHTTPClient trusts the panel's own certificate when the URL is https.
func panelHTTPClient(panelURL string) *http.Client {
	client := &http.Client{Timeout: 20 * time.Second}
	caFile := panelCAFile(panelURL)
	if caFile == "" {
		return client
	}
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return client
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pem) {
		return client
	}
	client.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
	}
	return client
}

// localAdminToken signs an access token for one call, valid for a minute.
//
// This is a local root escape hatch and is written down as one. It signs with
// the panel's own JWT secret, read from a file only root can read, on behalf of
// an administrator that already exists in the database - so the enrolment the
// panel records is attributed to a real account rather than to nobody. It grants
// nothing that the reader of that file did not already have: root on this host
// also holds the CA private key, the database password and the panel's TLS key.
//
// It exists because the alternative is an unattended installer that asks for a
// password, and the alternative to that is an installer that finishes with
// nothing managed. An operator who would rather not have it can pass
// --enrolment-token and this function is never reached.
func localAdminToken() (string, error) {
	secret := strings.TrimSpace(panelEnv("VKAI_JWT_SECRET", ""))
	if len(secret) < 32 {
		return "", fmt.Errorf("VKAI_JWT_SECRET is missing or too short in %s. Mint a token in the panel "+
			"(Servers -> Add agent) and pass --enrolment-token instead", config.EnvFile())
	}

	db, err := openPanelDB()
	if err != nil {
		return "", err
	}
	defer db.Close()

	var idStr, tenantStr, username, email string
	err = db.QueryRow(`
		SELECT u.id::text, u.tenant_id::text, u.username, u.email
		  FROM users u
		  JOIN user_roles ur ON ur.user_id = u.id
		  JOIN roles r       ON r.id = ur.role_id
		 WHERE u.deleted_at IS NULL
		   AND u.status = 'active'
		   AND lower(r.name) IN ('super_admin', 'super admin', 'superadmin', 'admin', 'platform_admin')
		 ORDER BY u.created_at
		 LIMIT 1`).Scan(&idStr, &tenantStr, &username, &email)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("this panel has no active administrator, so no enrolment could be attributed to one")
	}
	if err != nil {
		return "", err
	}

	userID, err := uuid.Parse(idStr)
	if err != nil {
		return "", err
	}
	tenantID, err := uuid.Parse(tenantStr)
	if err != nil {
		return "", err
	}
	roles, err := roleNamesFor(db, idStr)
	if err != nil {
		return "", err
	}

	manager := auth.NewJWTManager(secret, time.Minute, time.Minute, panelEnv("VKAI_JWT_ISSUER", "vkai-panel"))
	pair, err := manager.GenerateTokenPair(userID, tenantID, username, email, roles)
	if err != nil {
		return "", err
	}
	return pair.AccessToken, nil
}

func roleNamesFor(db *sqlx.DB, userID string) ([]string, error) {
	rows, err := db.Query(`
		SELECT r.name FROM roles r
		  JOIN user_roles ur ON ur.role_id = r.id
		 WHERE ur.user_id = $1::uuid`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// ============================================================
// LIST
// ============================================================

// runNodeList reads the nodes with the columns named one by one rather than with
// SELECT *, so a column added or not yet migrated in either direction cannot
// stop an operator from seeing what this panel manages. It deliberately does not
// build the server service: listing must not register anything.
func runNodeList(cmd *cobra.Command, args []string) {
	loadPanelEnv()

	db, err := openPanelDB()
	if err != nil {
		printError("Cannot open the panel database: %v", err)
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT id::text, hostname,
		       COALESCE(ip_address, ''), COALESCE(os, ''),
		       COALESCE(cpu_cores, 0), COALESCE(ram_total, 0),
		       COALESCE(role, ''), COALESCE(status, ''), COALESCE(agent_status, ''),
		       last_seen_at
		  FROM servers
		 WHERE deleted_at IS NULL
		 ORDER BY created_at`)
	if err != nil {
		printError("Cannot list the nodes: %v", err)
	}
	defer rows.Close()

	// Which row, if any, is the machine this command is running on. Read from
	// the identity file rather than guessed from the hostname; an unreadable or
	// absent file simply means the column stays empty.
	localID := ""
	if identity, idErr := localnode.LoadIdentity(config.EtcRoot()); idErr == nil {
		localID = identity.NodeID.String()
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tHOSTNAME\tADDRESS\tROLE\tSTATUS\tAGENT\tCPU\tRAM\tLAST SEEN\tTHIS HOST")
	fmt.Fprintln(w, "--\t--------\t-------\t----\t------\t-----\t---\t---\t---------\t---------")

	count, local := 0, 0
	for rows.Next() {
		var id, hostname, ip, osName, role, status, agentStatus string
		var cpuCores, ramTotal int64
		var lastSeen sql.NullTime
		if err := rows.Scan(&id, &hostname, &ip, &osName, &cpuCores, &ramTotal,
			&role, &status, &agentStatus, &lastSeen); err != nil {
			printError("Cannot read a node row: %v", err)
		}
		here := ""
		if localID != "" && id == localID {
			here = "yes"
			local++
		}
		seen := "-"
		if lastSeen.Valid {
			seen = lastSeen.Time.Local().Format("2006-01-02 15:04")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
			shortID(id), orDash(hostname), orDash(ip), orDash(role), orDash(status),
			orDash(agentStatus), cpuCores, humanBytes(ramTotal), seen, here)
		count++
	}
	if err := rows.Err(); err != nil {
		printError("Cannot read the node list: %v", err)
	}
	w.Flush()

	fmt.Printf("\nTotal: %d node(s)\n", count)
	switch {
	case count == 0:
		fmt.Println("This panel manages nothing yet. Make the machine it runs on its first node with:")
		fmt.Println("  sudo vkai node register")
	case local == 0:
		fmt.Println("None of these is the machine this panel runs on. Register it with:")
		fmt.Println("  sudo vkai node register")
	}
}

// ============================================================
// CONFIGURATION AND SMALL HELPERS
// ============================================================

var panelEnvLoaded = false

// loadPanelEnv makes the panel's own configuration visible to this process. The
// systemd units read /vkai-panel/etc/.env; a command run from a shell does not,
// and would otherwise reach for a database password that has not been right
// since the installer generated one. Values already in the environment win, so
// an operator can still override anything for a single invocation.
func loadPanelEnv() {
	if panelEnvLoaded {
		return
	}
	panelEnvLoaded = true

	raw, err := os.ReadFile(config.EnvFile())
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, strings.Trim(strings.TrimSpace(value), `"'`))
	}
}

func panelEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// openPanelDB opens the panel database from the panel's own configuration
// rather than from a compiled-in default. sqlx, because that is what the
// repository layer is built on.
func openPanelDB() (*sqlx.DB, error) {
	loadPanelEnv()
	if dsn := strings.TrimSpace(os.Getenv("VKAI_DATABASE_URL")); dsn != "" {
		return sqlx.Connect("pgx", dsn)
	}
	dsn := fmt.Sprintf("host=%s port=%s dbname=%s user=%s sslmode=%s",
		panelEnv("VKAI_DB_HOST", "127.0.0.1"),
		panelEnv("VKAI_DB_PORT", "5432"),
		panelEnv("VKAI_DB_NAME", "vkai_panel"),
		panelEnv("VKAI_DB_USER", "vkai"),
		panelEnv("VKAI_DB_SSLMODE", "disable"))
	if pass := panelEnv("VKAI_DB_PASSWORD", ""); pass != "" {
		dsn += " password=" + pass
	}
	return sqlx.Connect("pgx", dsn)
}

func isFalse(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "0", "false", "no", "off":
		return true
	default:
		return false
	}
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func orDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func truncateText(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func humanBytes(n int64) string {
	if n <= 0 {
		return "-"
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for size := n / unit; size >= unit; size /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func printWarn(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "! "+msg+"\n", args...)
}
