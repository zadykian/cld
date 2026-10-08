// Prints Ghostty's terminfo entry, xterm-ghostty, as Ghostty's build writes it: build-lib copies
// this file into Ghostty's src/ and runs it there, without building the app.
const std = @import("std");
const ghostty = @import("terminfo/ghostty.zig").ghostty;

pub fn main(init: std.process.Init) !void {
    var buffer: [1024]u8 = undefined;
    var out = std.Io.File.stdout().writerStreaming(init.io, &buffer);
    try ghostty.encode(&out.interface);
    try out.end();
}
