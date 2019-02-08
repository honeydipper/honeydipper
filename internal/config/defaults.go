package config

// Defaults contains the default mapping of features and drivers.
// can be overrided through configuration file in "drivers.daemon.featureMap".
var Defaults = map[string]string{
	"eventbus": "redisqueue",
}

// RequiredFeatures defines a list of features that are required to be loaded by the services.
var RequiredFeatures = map[string]([]string){
	"receiver": []string{
		"eventbus",
	},
	"operator": []string{
		"eventbus",
	},
	"engine": []string{
		"eventbus",
	},
}

// BuiltinDrivers defines a list of builtin feature drivers.
var BuiltinDrivers = map[string]DriverMeta{
	"redisqueue": {
		Name:     "redisqueue",
		Feature:  "eventbus",
		Services: []string{"engine", "receiver"},
		Data: map[string]interface{}{
			"Type":    "go",
			"Package": "github.com/honeyscience/honeydipper/drivers/cmd/redisqueue",
		},
	},
}
