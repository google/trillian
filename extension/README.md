# Extensions

Trillian defines a number of extension points to allow for customization by
forks. At runtime, implementations are acquired via an [extension.Registry](
https://github.com/google/trillian/blob/master/extension/registry.go), which
contains the comprehensive list of all supported extensions (bar the following).
