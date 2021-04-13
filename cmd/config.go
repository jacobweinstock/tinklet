package cmd

// configuration for Tinklet, values are normally set from flags, env, and file
type configuration struct {
	LogLevel   string `flag:"loglevel"`
	Config     string
	Identifier string `valid:"required"`
	TinkServer string `flag:"tinkServer" yaml:"tinkServer" valid:"required"`
	Registry   struct {
		Name string
		User string
		Pass string
	}
}
