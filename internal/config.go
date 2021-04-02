package internal

// Configuration for Tinklet, values are normally set from flags, env, and file
type Configuration struct {
	LogLevel   string `flag:"loglevel"`
	Config     string
	Identifier string
	TinkServer string `flag:"tinkServer" yaml:"tinkServer" valid:"required"`
	Registry   struct {
		Name string
		User string
		Pass string
	}
}
