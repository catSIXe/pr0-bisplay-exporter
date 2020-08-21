package settings

import (
	"os"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

// App defines the Application Settings structure
type App struct {
	PrometheusExporter           byte
	Username                     string
	Cookie                       string
	TargetIP                     string
	SettingNotificationFlash     byte
	SettingOnlyBenis             byte
	SettingHideTrend             byte
	SettingHideHochladID         byte
	SettingHideNotificationCount byte
	Setting5                     byte
	Setting6                     byte
	Setting7                     byte
}

// LoadSettings will pull the application config from the environment, or from
// a .env file
func LoadSettings() (config *App, err error) {
	config = &App{}

	if err = godotenv.Load(); err != nil {
		// We don't care if an .env is missing, it will be in prod.
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	if err = envconfig.Process("pr0stats", config); err != nil {
		return nil, err
	}

	return config, nil
}
