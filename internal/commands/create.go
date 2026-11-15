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
	Run: func(cmd *cobra.Command, args []string) {
		runCreate(args)
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

func runCreate(args []string) {
	var dbName string
	var provider string
	var version string
	var port int
	var rootPassword string
	var createUser bool
	var username string
	var userPassword string

	// Interactive mode: every prompt cancels with Esc.
	if createInteractive || len(args) == 0 {
		stateMgr, err := state.NewManager()
		if err != nil {
			color.Red("Error: %v", err)
			return
		}

		var ok bool
		dbName, ok, err = askText("Database name:", "", false, dbNameValidator(stateMgr))
		if err != nil {
			color.Red("Error: %v", err)
			return
		}
		if !ok {
			color.Yellow("Cancelled.")
			return
		}

		if createProvider == "" {
			choice, ok := promptSelect("Select database provider:", []string{utils.ProviderMySQL, utils.ProviderPostgres})
			if !ok {
				color.Yellow("Cancelled.")
				return
			}
			provider = choice
		} else {
			provider = createProvider
		}

		if createVersion == "" {
			v, ok := chooseVersion(provider)
			if !ok {
				color.Yellow("Cancelled.")
				return
			}
			version = v
		} else {
			version = createVersion
		}

		defaultPort := providers.DefaultMySQLPort
		if provider == utils.ProviderPostgres {
			defaultPort = providers.DefaultPostgresPort
		}
		if createPort != 0 {
			defaultPort = createPort
		}

		// Make the port choice obvious: list what is taken and suggest the
		// next free port when the default is busy.
		if summary := managedPortSummary(stateMgr); summary != "" {
			color.Yellow("Ports already used by managed databases: %s", summary)
		}
		suggest := defaultPort
		note := ""
		if managedPortOwner(stateMgr, defaultPort) != "" || portInUse(defaultPort) {
			suggest = nextFreePort(stateMgr, defaultPort)
			note = fmt.Sprintf(" (%d is busy)", defaultPort)
		}

		portStr, ok, err := askText(fmt.Sprintf("Port%s:", note), strconv.Itoa(suggest), false, portValidator(stateMgr))
		if err != nil {
			color.Red("Error: %v", err)
			return
		}
		if !ok {
			color.Yellow("Cancelled.")
			return
		}
		port, _ = strconv.Atoi(portStr) // the validator guarantees a number

		group, ok, err := askText("Group (Enter for global):", createGroup, false, nil)
		if err != nil {
			color.Red("Error: %v", err)
			return
		}
		if !ok {
			color.Yellow("Cancelled.")
			return
		}
		createGroup = group

		rootPassword, ok, err = askPassword("Root password:", requiredText)
		if err != nil {
			color.Red("Error: %v", err)
			return
		}
		if !ok {
			color.Yellow("Cancelled.")
			return
		}

		createUser, ok, err = askConfirm("Create a dedicated user?", true)
		if err != nil {
			color.Red("Error: %v", err)
			return
		}
		if !ok {
			color.Yellow("Cancelled.")
			return
		}

		if createUser {
			username, ok, err = askText("Username:", defaultUsername, false, identifierValidator)
			if err != nil {
				color.Red("Error: %v", err)
				return
			}
			if !ok {
				color.Yellow("Cancelled.")
				return
			}

			userPassword, ok, err = askPassword("User password:", requiredText)
			if err != nil {
				color.Red("Error: %v", err)
				return
			}
			if !ok {
				color.Yellow("Cancelled.")
				return
			}
		}
	} else {
		dbName = args[0]
		provider = createProvider
		version = createVersion
		port = createPort

		if provider == "" {
			color.Red("Error: --provider is required")
			return
		}

		var err error
		rootPassword, err = utils.GeneratePassword(utils.DefaultPasswordLength)
		if err != nil {
			color.Red("Error generating password: %v", err)
			return
		}

		userPassword, err = utils.GeneratePassword(utils.DefaultPasswordLength)
		if err != nil {
			color.Red("Error generating password: %v", err)
			return
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

	stateMgr, err := state.NewManager()
	if err != nil {
		color.Red("Error: %v", err)
		return
	}

	if !createInteractive && len(args) != 0 && stateMgr.Exists(dbName) {
		color.Red("Error: Database '%s' already exists", dbName)
		return
	}

	prov, err := providers.For(provider)
	if err != nil {
		color.Red("Error: %v", err)
		return
	}

	if port == 0 {
		port = prov.GetDefaultPort()
	}

	// Refuse ports taken by another managed database or by any local process.
	if owner := managedPortOwner(stateMgr, port); owner != "" {
		color.Red("Error: Port %d is already used by database '%s'", port, owner)
		return
	}
	if portInUse(port) {
		color.Red("Error: Port %d is already in use by another process", port)
		return
	}

	color.Cyan("\nCreating %s database '%s'...\n", provider, dbName)

	ctx := context.Background()
	dockerMgr, err := container.NewDockerManager(runtimeMode)
	if err != nil {
		color.Red("Error: %v", err)
		return
	}

	imageName := prov.GetImageName(version)
	color.White("Pulling image %s...", imageName)
	if err := dockerMgr.PullImage(ctx, imageName); err != nil {
		color.Red("Error pulling image: %v", err)
		return
	}

	containerName := fmt.Sprintf("xmdb_%s", dbName)
	volumeName := ""
	var volumePath string
	if createWithVolume {
		volumeName = fmt.Sprintf("xmdb_%s_data", dbName)

		if provider == utils.ProviderMySQL {
			volumePath = "/var/lib/mysql"
		} else {
			volumePath = "/var/lib/postgresql/data"
		}

		if exists, err := dockerMgr.VolumeExists(ctx, volumeName); err == nil && exists {
			// Images skip first-time init on an existing volume, so the new
			// credentials would never apply and creation would stall. Refuse
			// instead of hanging.
			color.Red("Error: volume '%s' already holds data from a previous database.", volumeName)
			fmt.Printf("  Remove it if you do not need the data:\n    docker volume rm %s\n  Or create without persistent storage: xmdb create <name> --volume=false\n", volumeName)
			return
		}
	}

	dbConfig := &providers.DBConfig{
		Host:         "localhost",
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
		Host:         "localhost",
		Port:         port,
		RootPassword: rootPassword,
		DatabaseName: dbName,
	}

	color.White("Creating container...")
	containerID, err := dockerMgr.CreateContainer(ctx, containerConfig)
	if err != nil {
		color.Red("Error creating container: %v", err)
		return
	}

	color.White("Starting container...")
	if err := dockerMgr.StartContainer(ctx, containerID, containerName); err != nil {
		color.Red("Error starting container: %v", err)
		cleanup(dockerMgr, ctx, containerID, containerName, volumeName)
		return
	}

	// Wait until the server answers a real protocol handshake. Rootless
	// container runtimes accept host-side TCP connections before the database
	// listens, so a bare port dial is not proof of readiness.
	color.White("Waiting for database to accept connections...")
	if err := waitForDatabase(ctx, prov, adminConfig, containerStartTimeout); err != nil {
		color.Red("Error: %v", err)
		cleanup(dockerMgr, ctx, containerID, containerName, volumeName)
		return
	}

	// Create user if requested. Fail hard instead of saving credentials that
	// do not exist.
	if createUser && username != "" {
		color.White("Creating user '%s'...", username)

		db, err := prov.Connect(adminConfig)
		if err != nil {
			color.Red("Error connecting to database: %v", err)
			cleanup(dockerMgr, ctx, containerID, containerName, volumeName)
			return
		}

		if err := prov.CreateUser(ctx, db, username, userPassword, dbName); err != nil {
			db.Close()
			color.Red("Error creating user: %v", err)
			cleanup(dockerMgr, ctx, containerID, containerName, volumeName)
			return
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
		color.Red("Error saving state: %v", err)
		return
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
}

// waitForDatabase polls until the server completes a protocol-level
// handshake. Each attempt is bounded by its own timeout, so a hung dial
// cannot stall past the overall deadline.
func waitForDatabase(ctx context.Context, prov providers.Provider, cfg *providers.DBConfig, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for {
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
			return fmt.Errorf("timeout after %v waiting for database (last error: %w)", timeout, lastErr)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
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

// managedPortSummary lists the ports held by managed databases with their
// names, e.g. "3306 (app), 5432 (analytics)".
func managedPortSummary(m *state.Manager) string {
	dbs, err := m.List()
	if err != nil {
		return ""
	}
	sort.Slice(dbs, func(i, j int) bool { return dbs[i].Port < dbs[j].Port })
	parts := make([]string, 0, len(dbs))
	for _, d := range dbs {
		parts = append(parts, fmt.Sprintf("%d (%s)", d.Port, d.Name))
	}
	return strings.Join(parts, ", ")
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

// chooseVersion lets the user pick one of the known Docker tags or enter any
// other tag manually.
func chooseVersion(provider string) (string, bool) {
	options := providers.KnownVersions(provider)
	options = append(options, "Type a version manually")

	choice, ok := promptSelect("Database version:", options)
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
