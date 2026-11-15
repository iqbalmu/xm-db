package commands

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/iqbalmu/xm-db/internal/container"
	"github.com/iqbalmu/xm-db/internal/providers"
	"github.com/iqbalmu/xm-db/internal/state"
	"github.com/iqbalmu/xm-db/pkg/utils"
	"github.com/spf13/cobra"
)

const (
	defaultUsername       = "dev_user"
	containerStartTimeout = 60 * time.Second
	portProbeTimeout      = 500 * time.Millisecond
	connectAttemptTimeout = 5 * time.Second
)

var (
	createProvider    string
	createVersion     string
	createPort        int
	createGroup       string
	createInteractive bool
	createWithVolume  bool
)

var createCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new database",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := runCreate(args)
		return err
	},
}

func init() {
	createCmd.Flags().StringVarP(&createProvider, "provider", "p", "", "Database provider (mysql, postgres)")
	createCmd.Flags().StringVarP(&createVersion, "version", "v", "", "Database version")
	createCmd.Flags().IntVar(&createPort, "port", 0, "Host port")
	createCmd.Flags().StringVarP(&createGroup, "group", "g", "", "Group the database belongs to (empty = global)")
	createCmd.Flags().BoolVarP(&createInteractive, "interactive", "i", false, "Interactive mode")
	createCmd.Flags().BoolVar(&createWithVolume, "volume", true, "Create with persistent volume (default: true)")
}

func runCreate(args []string) (bool, error) {
	var dbName string
	var provider string
	var version string
	var port int
	var rootPassword string
	var createUser bool
	var username string
	var userPassword string

	stateMgr, err := state.NewManager()
	if err != nil {
		return true, err
	}

	// Interactive mode: Esc goes back one prompt; Esc at the very first
	// prompt backs out of create entirely (reported to the caller).
	if createInteractive || len(args) == 0 {
		if !promptCreate(stateMgr, &dbName, &provider, &version, &port,
			&rootPassword, &createUser, &username, &userPassword) {
			return false, nil // backed out
		}
	} else {
		dbName = args[0]
		provider = createProvider
		version = createVersion
		port = createPort

		if provider == "" {
			return true, fmt.Errorf("--provider is required")
		}

		var err error
		rootPassword, err = utils.GeneratePassword(utils.DefaultPasswordLength)
		if err != nil {
			return true, fmt.Errorf("generating password: %w", err)
		}

		userPassword, err = utils.GeneratePassword(utils.DefaultPasswordLength)
		if err != nil {
			return true, fmt.Errorf("generating password: %w", err)
		}

		username = defaultUsername
		createUser = true

		if version == "" {
			version = providers.DefaultMySQLVersion
			if provider == utils.ProviderPostgres {
				version = providers.DefaultPostgresVersion
			}
		}
	}

	if !createInteractive && len(args) != 0 && stateMgr.Exists(dbName) {
		return true, fmt.Errorf("database '%s' already exists", dbName)
	}

	prov, err := providers.For(provider)
	if err != nil {
		return true, err
	}

	if port == 0 {
		port = prov.GetDefaultPort()
	}

	// Refuse ports taken by another managed database or by any local process.
	if owner := managedPortOwner(stateMgr, port); owner != "" {
		return true, fmt.Errorf("port %d is already used by database '%s'", port, owner)
	}
	if portInUse(port) {
		return true, fmt.Errorf("port %d is already in use by another process", port)
	}

	color.Cyan("\nCreating %s database '%s'...\n", provider, dbName)

	ctx := context.Background()
	dockerMgr, err := container.NewDockerManager(runtimeMode)
	if err != nil {
		return true, err
	}

	imageName := prov.GetImageName(version)
	color.White("Pulling image %s...", imageName)
	if err := dockerMgr.PullImage(ctx, imageName); err != nil {
		return true, fmt.Errorf("pulling image: %w", err)
	}

	containerName := fmt.Sprintf("xmdb_%s", dbName)
	volumeName := ""
	var volumePath string
	if createWithVolume {
		volumeName = fmt.Sprintf("xmdb_%s_data", dbName)
		volumePath = prov.GetVolumePath(version)

		if exists, err := dockerMgr.VolumeExists(ctx, volumeName); err == nil && exists {
			// Images skip first-time init on an existing volume, so the new
			// credentials would never apply and creation would stall. Refuse
			// instead of hanging.
			return true, fmt.Errorf("volume '%s' already holds data from a previous database; remove it with 'docker volume rm %s' or pass --volume=false", volumeName, volumeName)
		}
	}

	dbConfig := &providers.DBConfig{
		Host:         "127.0.0.1",
		Port:         port,
		RootPassword: rootPassword,
		DatabaseName: dbName,
		User:         username,
		UserPassword: userPassword,
	}

	containerConfig := &container.ContainerConfig{
		Name:          containerName,
		Image:         imageName,
		Port:          port,
		ContainerPort: prov.GetContainerPort(),
		Env:           prov.GetEnvVars(dbConfig),
		VolumeName:    volumeName,
		VolumePath:    volumePath,
	}

	// Admin connection for readiness checks and user provisioning: the dev
	// user does not exist until CreateUser runs.
	adminConfig := &providers.DBConfig{
		Host:         "127.0.0.1",
		Port:         port,
		RootPassword: rootPassword,
		DatabaseName: dbName,
	}

	color.White("Creating container...")
	containerID, err := dockerMgr.CreateContainer(ctx, containerConfig)
	if err != nil {
		return true, fmt.Errorf("creating container: %w", err)
	}

	color.White("Starting container...")
	if err := dockerMgr.StartContainer(ctx, containerID, containerName); err != nil {
		cleanup(dockerMgr, ctx, containerID, containerName, volumeName)
		return true, fmt.Errorf("starting container: %w", err)
	}

	// Wait until the server answers a real protocol handshake. Rootless
	// container runtimes accept host-side TCP connections before the database
	// listens, so a bare port dial is not proof of readiness.
	color.White("Waiting for database to accept connections...")
	if err := waitForDatabase(ctx, dockerMgr, containerID, containerName, prov, adminConfig, containerStartTimeout); err != nil {
		cleanup(dockerMgr, ctx, containerID, containerName, volumeName)
		return true, err
	}

	// Create user if requested. Fail hard instead of saving credentials that
	// do not exist.
	if createUser && username != "" {
		color.White("Creating user '%s'...", username)

		db, err := prov.Connect(adminConfig)
		if err != nil {
			cleanup(dockerMgr, ctx, containerID, containerName, volumeName)
			return true, fmt.Errorf("connecting to database: %w", err)
		}

		if err := prov.CreateUser(ctx, db, username, userPassword, dbName); err != nil {
			db.Close()
			cleanup(dockerMgr, ctx, containerID, containerName, volumeName)
			return true, fmt.Errorf("creating user: %w", err)
		}
		db.Close()
	}

	dbState := &state.Database{
		Name:          dbName,
		Provider:      provider,
		Version:       version,
		Group:         strings.TrimSpace(createGroup),
		ContainerName: containerName,
		ContainerID:   containerID,
		Port:          port,
		RootPassword:  rootPassword,
		DatabaseName:  dbName,
		User:          username,
		UserPassword:  userPassword,
		VolumeName:    volumeName,
		CreatedAt:     time.Now(),
	}

	if err := stateMgr.Save(dbState); err != nil {
		return true, fmt.Errorf("saving state: %w", err)
	}

	color.Green("\nDatabase '%s' created successfully!\n", dbName)

	fmt.Println("Connection Information:")
	color.Cyan("  Provider: %s", provider)
	color.Cyan("  Port: %d", port)
	color.Cyan("  Database: %s", dbName)

	if createUser && username != "" {
		color.Cyan("  User: %s", username)
		color.Cyan("  Password: %s", userPassword)
		fmt.Println()

		color.White("Connection String:")
		fmt.Printf("  %s\n", prov.GetConnectionString(dbConfig))
	}

	fmt.Println()
	return true, nil
}

// Steps of the guided create flow, in prompt order.
const (
	stepName = iota
	stepProvider
	stepVersion
	stepPort
	stepGroup
	stepRootPassword
	stepAskUser
	stepUsername
	stepUserPassword
	stepDone
)

// promptCreate runs the guided flow as a step machine. Esc returns to the
// previous prompt; Esc at the very first prompt returns false, which hands
// control back to the wizard menu. Values already entered are kept and
// re-offered as defaults when navigating back.
func promptCreate(
	stateMgr *state.Manager,
	dbName, provider, version *string,
	port *int,
	rootPassword *string,
	createUser *bool,
	username, userPassword *string,
) bool {
	n, p, v, pt, grp, rp, cu, u, up, ok := runCreateTUI(stateMgr)
	if !ok {
		return false
	}
	*dbName = n
	if createProvider != "" {
		*provider = createProvider
	} else {
		*provider = p
	}
	if createVersion != "" {
		*version = createVersion
	} else {
		*version = v
	}
	if createPort != 0 {
		*port = createPort
	} else {
		*port = pt
	}
	if createGroup == "" {
		createGroup = grp
	}
	*rootPassword = rp
	*createUser = cu
	*username = u
	*userPassword = up
	return true
}

func promptCreateLegacy(
	stateMgr *state.Manager,
	dbName, provider, version *string,
	port *int,
	rootPassword *string,
	createUser *bool,
	username, userPassword *string,
) bool {
	for step := stepName; step != stepDone; {
		back := false

		switch step {
		case stepName:
			v, ok, err := askText("Database name:", *dbName, false, dbNameValidator(stateMgr))
			if err != nil {
				color.Red("Error: %v", err)
				return true
			}
			back = !ok
			if !back {
				*dbName = v
			}

		case stepProvider:
			if createProvider != "" {
				*provider = createProvider
				break
			}
			choice, ok := promptSelectDefault("Select database provider:", []string{utils.ProviderMySQL, utils.ProviderPostgres}, *provider)
			back = !ok
			if !back {
				*provider = choice
			}

		case stepVersion:
			if createVersion != "" {
				*version = createVersion
				break
			}
			v, ok := chooseVersion(*provider)
			back = !ok
			if !back {
				*version = v
			}

		case stepPort:
			defaultPort := providers.DefaultMySQLPort
			if *provider == utils.ProviderPostgres {
				defaultPort = providers.DefaultPostgresPort
			}
			if createPort != 0 {
				defaultPort = createPort
			}

			suggest := defaultPort
			if managedPortOwner(stateMgr, defaultPort) != "" || portInUse(defaultPort) {
				suggest = nextFreePort(stateMgr, defaultPort)
			}
			showPortAvailability(stateMgr, defaultPort, suggest)

			def := strconv.Itoa(suggest)
			if *port != 0 {
				def = strconv.Itoa(*port)
			}
			portStr, ok, err := askText("Port:", def, false, portValidator(stateMgr))
			if err != nil {
				color.Red("Error: %v", err)
				return true
			}
			back = !ok
			if !back {
				*port, _ = strconv.Atoi(portStr) // the validator guarantees a number
			}

		case stepGroup:
			if createGroup != "" {
				break // -g flag wins, like the provider and version flags
			}
			g, ok := chooseGroup(stateMgr, createGroup)
			back = !ok
			if !back {
				createGroup = g
			}

		case stepRootPassword:
			v, ok, err := askPassword("Root password:", requiredText)
			if err != nil {
				color.Red("Error: %v", err)
				return true
			}
			back = !ok
			if !back {
				*rootPassword = v
			}

		case stepAskUser:
			v, ok, err := askConfirm("Create a dedicated user?", true)
			if err != nil {
				color.Red("Error: %v", err)
				return true
			}
			back = !ok
			if !back {
				*createUser = v
			}

		case stepUsername:
			if !*createUser {
				break
			}
			v, ok, err := askText("Username:", *username, false, identifierValidator)
			if err != nil {
				color.Red("Error: %v", err)
				return true
			}
			back = !ok
			if !back {
				*username = v
			}

		case stepUserPassword:
			v, ok, err := askPassword("User password:", requiredText)
			if err != nil {
				color.Red("Error: %v", err)
				return true
			}
			back = !ok
			if !back {
				*userPassword = v
			}
		}

		if back {
			if step == stepName {
				return false // backed out of create entirely
			}
			step--
			continue
		}

		if step == stepAskUser && !*createUser {
			step = stepDone // no user prompts to visit
			continue
		}

		step++
	}
	return true
}

// waitForDatabase polls until the server completes a protocol-level
// handshake. Each attempt is bounded by its own timeout, so a hung dial
// cannot stall past the overall deadline. A container that already exited
// fails fast with its log tail instead of burning the whole timeout, and a
// timeout is reported with the log tail too.
func waitForDatabase(ctx context.Context, dockerMgr *container.DockerManager, containerID, containerName string, prov providers.Provider, cfg *providers.DBConfig, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for {
		if status, err := dockerMgr.ContainerStatus(ctx, containerID, containerName); err == nil && status == "exited" {
			logs, tailErr := dockerMgr.TailLogs(ctx, containerID, containerName, 20)
			if tailErr != nil || logs == "" {
				return fmt.Errorf("database container exited during startup (no logs available)")
			}
			return fmt.Errorf("database container exited during startup; last log lines:\n%s", indentBlock(logs))
		}

		attemptCtx, cancel := context.WithTimeout(ctx, connectAttemptTimeout)
		db, err := prov.Connect(cfg)
		if err != nil {
			lastErr = err
		} else {
			lastErr = db.PingContext(attemptCtx)
			db.Close()
		}
		cancel()
		if lastErr == nil {
			return nil
		}

		if time.Now().After(deadline) {
			logs, tailErr := dockerMgr.TailLogs(ctx, containerID, containerName, 10)
			if tailErr == nil && logs != "" {
				return fmt.Errorf("timeout after %v waiting for database (last error: %v); container log tail:\n%s", timeout, lastErr, indentBlock(logs))
			}
			return fmt.Errorf("timeout after %v waiting for database (last error: %w); inspect the container with: docker logs %s", timeout, lastErr, containerName)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// indentBlock prefixes every line so a log tail sits cleanly inside an error.
func indentBlock(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = "    " + lines[i]
	}
	return strings.Join(lines, "\n")
}

// cleanup removes a failed creation's container and data volume. Failures
// are reported, not swallowed: a stuck container blocks the next create
// with the same name.
func cleanup(dockerMgr *container.DockerManager, ctx context.Context, containerID, containerName, volumeName string) {
	if err := dockerMgr.RemoveContainer(ctx, containerID, containerName); err != nil {
		color.Yellow("  Warning: could not clean up container '%s': %v", containerName, err)
	}
	if volumeName != "" {
		if err := dockerMgr.RemoveVolume(ctx, volumeName); err != nil {
			color.Yellow("  Warning: could not clean up volume '%s': %v", volumeName, err)
		}
	}
}

// managedPortOwner returns the name of the managed database bound to port.
func managedPortOwner(m *state.Manager, port int) string {
	dbs, err := m.List()
	if err != nil {
		return ""
	}
	for _, d := range dbs {
		if d.Port == port {
			return d.Name
		}
	}
	return ""
}

// portInUse reports whether a local process is already listening on the port.
func portInUse(port int) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), portProbeTimeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

var (
	dbNameRe     = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,49}$`)
	versionRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	identifierRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,32}$`)
)

// dbNameValidator rejects names that would break container or volume naming,
// and names already taken by a managed database.
func dbNameValidator(m *state.Manager) func(string) error {
	return func(s string) error {
		if !dbNameRe.MatchString(s) {
			return errors.New("use letters, digits, '.', '_' or '-' (max 50, start alphanumeric)")
		}
		if m.Exists(s) {
			return fmt.Errorf("database '%s' already exists", s)
		}
		return nil
	}
}

// portValidator rejects non-numbers and ports held by a managed database or
// any other local process.
func portValidator(m *state.Manager) func(string) error {
	return func(s string) error {
		port, err := strconv.Atoi(s)
		if err != nil || port < 1 || port > 65535 {
			return errors.New("port must be a number between 1 and 65535")
		}
		if owner := managedPortOwner(m, port); owner != "" {
			return fmt.Errorf("port %d is already used by database '%s'", port, owner)
		}
		if portInUse(port) {
			return fmt.Errorf("port %d is already in use by another process", port)
		}
		return nil
	}
}

func versionValidator(s string) error {
	if !versionRe.MatchString(s) {
		return errors.New("use letters, digits, '.', '_' or '-'")
	}
	return nil
}

func identifierValidator(s string) error {
	if !identifierRe.MatchString(s) {
		return errors.New("use letters, digits or '_' (max 32)")
	}
	return nil
}

func requiredText(s string) error {
	if strings.TrimSpace(s) == "" {
		return errors.New("value is required")
	}
	return nil
}

// showPortAvailability prints the tidy port block shown before the port
// prompt: which ports the managed databases hold and which port is free.
func showPortAvailability(m *state.Manager, defaultPort, suggest int) {
	fmt.Println()
	color.Cyan("  Port availability")
	for _, row := range portAvailabilityRows(m, defaultPort, suggest) {
		fmt.Printf("  %s\n", row)
	}
	fmt.Println()
}

// portAvailabilityRows builds the aligned rows of that block.
func portAvailabilityRows(m *state.Manager, defaultPort, suggest int) []string {
	var rows []string
	seen := map[int]bool{}
	if dbs, err := m.List(); err == nil {
		sort.Slice(dbs, func(i, j int) bool { return dbs[i].Port < dbs[j].Port })
		for _, d := range dbs {
			rows = append(rows, fmt.Sprintf("%5d  %-6s  managed database '%s'", d.Port, "in use", d.Name))
			seen[d.Port] = true
		}
	}
	if defaultPort != suggest && !seen[defaultPort] && portInUse(defaultPort) {
		rows = append(rows, fmt.Sprintf("%5d  %-6s  another process", defaultPort, "in use"))
	}
	rows = append(rows, fmt.Sprintf("%5d  %-6s  suggested", suggest, "free"))
	return rows
}

// nextFreePort returns the first port from start upward that no managed
// database and no local process holds; start itself when nothing is free
// within 100 ports.
func nextFreePort(m *state.Manager, start int) int {
	for port := start; port < start+100; port++ {
		if managedPortOwner(m, port) == "" && !portInUse(port) {
			return port
		}
	}
	return start
}

// chooseVersion lets the user pick one of the known Docker tags (newest
// first; the configured default is pre-highlighted) or enter any other tag
// manually.
func chooseVersion(provider string) (string, bool) {
	options := providers.KnownVersions(provider)
	options = append(options, "Type a version manually")

	def := providers.DefaultMySQLVersion
	if provider == utils.ProviderPostgres {
		def = providers.DefaultPostgresVersion
	}

	choice, ok := promptSelectDefault("Database version:", options, def)
	if !ok {
		return "", false
	}
	if choice != "Type a version manually" {
		return choice, true
	}

	version, ok, err := askText("Version:", "", false, versionValidator)
	if err != nil {
		color.Red("Error: %v", err)
		return "", false
	}
	if !ok {
		return "", false
	}
	return version, true
}

// knownGroups returns the distinct group names already used by managed
// databases, sorted alphabetically.
func knownGroups(m *state.Manager) []string {
	dbs, err := m.List()
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var groups []string
	for _, d := range dbs {
		if d.Group == "" || seen[d.Group] {
			continue
		}
		seen[d.Group] = true
		groups = append(groups, d.Group)
	}
	sort.Strings(groups)
	return groups
}

var groupRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,29}$`)

func groupValidator(s string) error {
	if !groupRe.MatchString(strings.TrimSpace(s)) {
		return errors.New("use letters, digits, '.', '_', '-' or spaces (max 30)")
	}
	return nil
}

// chooseGroup lets the user keep the database ungrouped, pick a group that
// already exists, or type a new one.
func chooseGroup(m *state.Manager, current string) (string, bool) {
	options := append([]string{globalGroupLabel}, knownGroups(m)...)
	options = append(options, "Type a new group manually")

	choice, ok := promptSelectDefault("Group:", options, current)
	if !ok {
		return "", false
	}
	if choice == "Type a new group manually" {
		v, ok, err := askText("Group:", "", false, groupValidator)
		if err != nil {
			color.Red("Error: %v", err)
			return "", false
		}
		if !ok {
			return "", false
		}
		choice = strings.TrimSpace(v)
	}
	if choice == globalGroupLabel {
		return "", true // global = ungrouped
	}
	return choice, true
}
