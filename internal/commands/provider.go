package commands

import (
	"github.com/iqbalmu/dbm/internal/providers"
	"github.com/iqbalmu/dbm/internal/state"
)

// connectionConfig builds the DBConfig used to connect to a database from host.
func connectionConfig(db *state.Database) *providers.DBConfig {
	return &providers.DBConfig{
		Host:         "localhost",
		Port:         db.Port,
		RootPassword: db.RootPassword,
		DatabaseName: db.DatabaseName,
		User:         db.User,
		UserPassword: db.UserPassword,
	}
}
