package main

// Defaults : contains the default settings of the system
var Defaults = map[string]string{
	"eventbus": "redis_pubsub",
}

// RequiredFeatures : define the required features that each service should provide
var RequiredFeatures = map[string]([]string){
	"receiver": []string{
		"eventbus",
	},
	"engine": []string{
		"eventbus",
	},
}

// BuiltinDrivers : define a list of builtin feature drivers
var BuiltinDrivers map[string]DriverMeta = map[string]DriverMeta{
	"google_pubsub": DriverMeta{
		Name:     "google_pubsub",
		Feature:  "eventbus",
		Services: []string{"engine"},
		Data: map[string]interface{}{
			"Type":    "go",
			"Package": "github.com/honeyscience/honeydipper/honeydipper-googlepubsub",
		},
	},
	"redis_pubsub": DriverMeta{
		Name:     "redis_pubsub",
		Feature:  "eventbus",
		Services: []string{"engine", "receiver"},
		Data: map[string]interface{}{
			"Type":    "go",
			"Package": "github.com/honeyscience/honeydipper/honeydipper-redispubsub",
		},
	},
}
