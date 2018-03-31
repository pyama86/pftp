package pftp

type Middleware interface {
	User(string) (string, error)
}
