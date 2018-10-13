package main

func startReceiver(cfg *Config) {
	Services["receiver"] = NewService(cfg, "engine")
	Services["receiver"].start()
}
