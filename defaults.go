package main

// Defaults : contains the default settings of the system
var Defaults = map[string]string{
	"eventbus": "redis_pubsub",
}

// RequiredFunctions : define the required functions that each service should provide
var RequiredFunctions = map[string]([]string){
	"receiver": []string{
		"eventbus",
	},
	"engine": []string{
		"eventbus",
	},
}

// BuiltinDrivers : define a list of builtin function drivers
var BuiltinDrivers map[string]DriverMeta = map[string]DriverMeta{
	"google_pubsub": DriverMeta{
		Name:   "google_pubsub",
		Lookup: "eventbus",
		Domain: []string{"engine"},
		Data: map[string]interface{}{
			"Package":    "github.com/honeyscience/honeydipper/googlePubSubDriver",
			"Executable": "honeydipper-googlepubsub",
			"Arguments":  []interface{}{},
		},
	},
	"redis_pubsub": DriverMeta{
		Name:   "redis_pubsub",
		Lookup: "eventbus",
		Domain: []string{"engine"},
		Data: map[string]interface{}{
			"Package":    "github.com/honeyscience/honeydipper/redisPubSubDriver",
			"Executable": "honeydipper-redispubsub",
			"Arguments":  []interface{}{},
		},
	},
}
