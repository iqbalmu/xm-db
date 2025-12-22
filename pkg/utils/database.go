package utils

// GetDriverName returns the database driver name for sql.Open()
func GetDriverName(provider string) string {
	if provider == "postgres" {
		return "postgres"
	}
	return "mysql"
}
