package main

func startEngine(cfg *Config) {
	Services["engine"] = NewService(cfg, "engine")
	Services["engine"].start()
}
