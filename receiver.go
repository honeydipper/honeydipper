package main

func startReceiver(cfg *Config) {
	Services["receiver"] = NewService(cfg, "receiver")
	go Services["receiver"].start()
}
