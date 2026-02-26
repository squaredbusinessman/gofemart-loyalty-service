package option

type Option[T any] func(*T)

func Apply[T any](target *T, opts ...Option[T]) {
	for _, opt := range opts {
		if opt != nil {
			opt(target)
		}
	}
}
