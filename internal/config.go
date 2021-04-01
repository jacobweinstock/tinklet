package internal

type Configuration struct {
	LogLevel   string `flag:"loglevel"`
	Config     string
	Identifier string
	TinkServer string `flag:"tinkServer" yaml:"tinkServer"`
	Registry   struct {
		Name string
		User string
		Pass string
	}
}
