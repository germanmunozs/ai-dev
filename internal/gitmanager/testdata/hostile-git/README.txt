Feature 008 hostile-Git fixtures use only synthetic sentinel values.

Tests construct configuration and environment entries that point at a local
marker command. If any forbidden helper, hook, filter, pager, editor, signing
command, fsmonitor, askpass program, or transport helper is invoked, that local
command records the invocation in a temporary directory owned by the test.

Never add real credentials, tokens, remote URLs, or user configuration here.
