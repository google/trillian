# Extensions

Trillian defines a number of extension points to allow for customization by
forks. At runtime, implementations are acquired via an [extension.Registry](
https://github.com/google/trillian/blob/master/extension/registry.go), which
contains the comprehensive list of all supported extensions.

## Default registry

The default registry is acquired via [builtin.NewDefaultExtensionRegistry](
https://github.com/google/trillian/blob/master/extension/builtin/default_registry.go).
It's configured to the standard Trillian implementation, backed by MySQL.

## Custom extensions

Here's a sample procedure of how to use custom extensions:

1. Create your own [extension.Registry](
   https://github.com/google/trillian/blob/master/extension/registry.go)
   implementation, customising as desired.

  * If you want to reuse some of the standard extensions, refer to
    [defaultRegistry](https://github.com/google/trillian/blob/master/extension/builtin/default_registry.go).

  * Consider wrapping your implementation with [extension.NewCachedRegistry](
    https://github.com/google/trillian/blob/master/extension/cached_registry.go)
    before use.

1. Either fork Trillian or duplicate the entry points you want to use in your
   repo.

1. [Replace calls to builtin.NewDefaultExtensionRegistry](
   https://github.com/google/trillian/search?utf8=%E2%9C%93&q=%22builtin.NewDefaultExtensionRegistry%28%29%22&type=Code)
   with your implementation.

1. Your customised Trillian is now ready for use.
