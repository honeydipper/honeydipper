// Copyright 2019 Honey Science Corporation
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, you can obtain one at http://mozilla.org/MPL/2.0/.

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
