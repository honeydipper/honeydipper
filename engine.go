package main

func startEngine(cfg *Config) {
	Services["engine"] = NewService(cfg, "engine")
	go Services["engine"].start()
}
