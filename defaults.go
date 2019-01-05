package main

// Defaults : contains the default settings of the system
var Defaults = map[string]string{
	"eventbus": "redisqueue",
}

// RequiredFeatures : define the required features that each service should provide
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

// BuiltinDrivers : define a list of builtin feature drivers
var BuiltinDrivers = map[string]DriverMeta{
	"redisqueue": DriverMeta{
		Name:     "redisqueue",
		Feature:  "eventbus",
		Services: []string{"engine", "receiver"},
		Data: map[string]interface{}{
			"Type":    "go",
			"Package": "github.com/honeyscience/honeydipper/drivers/redisqueue",
		},
	},
}
