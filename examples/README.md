# Ruby examples

Pure-Ruby examples for the `msgpack` library as provided by
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby) (rbgo). Run them
with the `rbgo` interpreter:

```sh
rbgo examples/msgpack_usage.rb
```

| File | Shows |
| --- | --- |
| [`msgpack_usage.rb`](msgpack_usage.rb) | Pack/unpack round-trips, `dump`/`load` aliases and `Object#to_msgpack`. |

Each example is executed as-is under rbgo (`require "msgpack"`).
