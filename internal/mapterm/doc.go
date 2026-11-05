// Package mapterm draws OpenStreetMap vector tiles as a terminal map. It holds
// the projection, tile maths, a dot canvas with line and fill drawing, and the
// tile source and cache. It never waits on the network inside a draw: callers
// fetch missing tiles in the background and draw again when they arrive.
package mapterm
