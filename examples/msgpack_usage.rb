# frozen_string_literal: true
#
# Pure-Ruby usage of the MessagePack module, as provided by go-embedded-ruby (rbgo).
# Run it with:  rbgo examples/msgpack_usage.rb

require "msgpack"

# Pack a value to the MessagePack wire format: a compact, binary (ASCII-8BIT) String.
packed = MessagePack.pack({ "name" => "dimail", "count" => 3, "active" => true })
puts packed.encoding.name          # => ASCII-8BIT
puts packed.bytesize               # => 28

# Unpack the bytes back into the original tree of Ruby values.
p MessagePack.unpack(packed)       # => {"name" => "dimail", "count" => 3, "active" => true}

# Arrays, nested structures, floats and nil all round-trip.
p MessagePack.unpack(MessagePack.pack([1, 2.5, ["x", nil]]))

# .dump / .load are aliases for .pack / .unpack.
p MessagePack.load(MessagePack.dump([42, "hi"]))

# Object#to_msgpack packs the receiver in place.
p MessagePack.unpack("hello".to_msgpack)
