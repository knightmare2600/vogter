# The map

Screen `5` draws OpenStreetMap in the terminal and places Zabbix hosts on it by their inventory location. This
document covers how it works, how to configure it, and the licences that apply.

## Example

Central Odense at zoom 16, 76 columns wide, in each mode. Map data © OpenStreetMap contributors, ODbL; tiles ©
OpenMapTiles, from OpenFreeMap.

Braille, the default:

```
⡜⡄   ⠐⢎⠁⠰⡁   ⢀⡠⡀  ⠑⡌⢢ ⠈⠢⡀ ⠋⠙ ⠈⡞⣇      ⣀⡠⠔⠓⠉⠉ ⢀⣀⠤⣒⠊⠉  ⠘⣆      ⣘⣄   ⠘⡄        
⠘⡌⢆   ⢐⡵⡉     ⢑⠜⢄  ⠈⠢⡑⢄ ⠑⢄    ⡱⡟⠢⠤⠒⠒⠉⢉Flakhaven  ⠑⠤⡀  ⠘⣆ ⢀⡠⠖⠉ ⠘⣆  ⢀⣸⢄   ⢀⣤⠶⠛
 ⠘⢌⢢  ⠈⠒⡕⢄     ⢣⢀⡹⠄  ⠘⢄⠑⡄ ⠑Asylgade⠉⣉⣁⡠⠤⠔⠒⢄⠈⢆      ⠈⠒⢄⡀⠘⢮⠁   ⢀⡠⠜⢶⠚⠉ ⠈⣦⠴⠚⠉   
 ⢀⠬⢆⢣   ⠈⠹⡢⡀    Lille Gråbrødrestræde     ⠈⢆⠘⢖⠤⠤⠤⣀⣀⣀  ⠈⠢⢌⢧⡠⠔⠊⠁ ⣀⠬⢧⡤⠖⠋⠁    ⢀⡠
⣎⠁ ⠈⢢⠱⡀  ⠈⠦⠓     ⣀⠤⠒⠉⢀⡠⠔⣊⡡⡄⣀⠤⠒⠉            ⠘⡄⠘⡄     ⠉⠉⠑⠒⠒⣻⢧⢀⡠⠔⠊   ⢣   ⢀⡠⠔⠊⠁ 
⠹⣆   ⢣⠑⡄     ⣀⠤⠒⠉⢀⡠⢔⡪⠕⠒⠉  ⠉                 ⠱⡀⠘⡄     ⣀⠤⠒⠉⢠⢻⢣   ⢀⡠⠔⠊⢣⠔⠊⠁    ⢀
Jernbanegade⠉⢀⡠⠔⣊⠕⠊⠁                         ⢣ ⠘⡄⣀⠤⠒⠉   ⢠⠃⢸ ⢳⡔⠊⠁    ⢣  ⢀⡠⠔⠊⠁
  ⠘⣧⠒⠃ ⠑⡄⢀⣠⣔⡪⠥⠒⠉                              ⢇ ⠹⡀     ⢠⠃ ⢸  ⢳⡀      ⢳⠊⠁    
```

Unicode:

```
·▒   ▒▒▒▒▒   ▒▒▒  ▒·· ▒▒▒ ▒▒ ▒··      ▒▒▒▒▒▒ ······  ··      ▒▒   ▒▒        
··▒   ▒▒▒     ▒▒▒  ▒▒·· ▒▒    ▒·▒▒▒▒▒·Flakhaven  ···  ·· ▒▒▒▒ ▒▒  ▒▒▒   ▒▒▒▒
 ··▒  ▒▒▒▒     ▒▒▒▒  ▒▒·· ▒Asylgade···▒▒▒▒▒··      ······▒   ▒▒▒▒▒▒ ▒▒▒▒▒   
 ···▒   ▒▒▒▒    Lille Gråbrødrestræde     ▒▒········  ····▒▒▒▒ ▒▒▒▒▒▒▒    ▒▒
·· ··▒▒  ▒▒▒     ▒▒▒▒·····▒▒▒▒▒            ▒▒··     ·······▒▒▒▒   ▒   ▒▒▒▒▒ 
··   ·▒▒     ▒▒▒▒·····▒▒  ▒                 ▒▒··     ·······   ▒▒▒▒▒▒▒▒    ·
Jernbanegade▒·····▒▒                         ▒ ······   ··· ·▒▒▒    ▒  ·····
  ▒··· ·······▒▒                              ▒ ··     ·· ·  ·▒      ···    
```

ASCII, for terminals without Unicode fonts. Names are spelt the Danish way: Gråbrødre becomes Graabroedre.

```
.#   #####   ###  #.. ### ## #..      ###### ......  ..      ##   ##        
..#   ###     ###  ##.. ##    #.#####.Flakhaven  ...  .. #### ##  ###   ####
 ..#  ####     ####  ##.. #Asylgade...#####..      ......#   ###### #####   
 ...#   ####  Lille Graabroedrestraede    ##........  ....#### #######    ##
.. ..##  ###     ####.....#####            ##..     .......####   #   ##### 
..   .##     ####.....##  #                 ##..     .......   ########    .
Jernbanegade#.....##                         # ......   ... .###    #  .....
  #... .......##                              # ..     .. .  .#      ...    
```

## Turning it on

The map is off by default. Drawing it sends the area you view, and your IP address, to OpenFreeMap and its CDN,
Cloudflare. Until the map is turned on, Vægter builds no network client for it at all. Press `e` on screen 5 to turn
it on for one session, or keep it on in the config:

```toml
[map]
enabled = true
render_mode = "braille"        # braille, unicode or ascii; v cycles
palette = "green"              # green, solarized-dark, solarized-light; P cycles
show_labels = true
show_buildings = true
show_railways = true
show_parks = true
cache_enabled = true
cache_directory = ""           # default: your user cache folder, under vogter/tiles
tile_source = "https://tiles.openfreemap.org/planet"
# Where it opens when no host has a location:
default_latitude = 55.39593945
default_longitude = 10.38620684
default_zoom = 12
```

Every setting is optional, and a config without `[map]` stays valid. `vaegter check` reports bad values.

## Keys

| Key | Action |
|---|---|
| `h j k l`, arrows | Pan by 15% of the screen |
| `PgUp` `PgDn`, `ctrl-u` `ctrl-d` | Pan a screen or half a screen up or down |
| `+` `-`, mouse wheel | Zoom; the wheel needs the mouse on (`M`), and `shift` with it pans |
| `:` | Go to `latitude, longitude[, zoom]` in decimal degrees |
| `b` | Back to the previous place, up to 32 steps |
| `f` | Fit every host shown, or return to the start |
| `n` `p` | Next or previous host, worst first; it blinks while selected |
| `c` | Centre on the selected host |
| `Enter` | Open the selected host's page; `esc` returns to the map |
| `x` | Only hosts with problems |
| `/` | Filter hosts by name or group |
| `v` | Drawing mode |
| `P` | Palette |
| `y` | Layers menu: buildings, railways, parks, water, boundaries |
| `L` | Names on or off |
| `o` | Open the same view on openstreetmap.org |

## Hosts on the map

Hosts come from `host.get` with `selectInventory`: `location_lat`, `location_lon`, `location`, `site_city` and
`site_country`. They are read once a minute, and only while the map or a board is on screen. Coordinates may use a
decimal point or a decimal comma (`55,39594`, as a Danish locale types it), a degree sign, or a compass letter
(`33.86S`). Zabbix returns `[]` for a host whose inventory is disabled, which Vægter treats as having no location.

A host with no coordinates can be placed in the config, by name or host ID, and the config wins over the inventory:

```toml
[profiles.prod.locations]
"web-01" = [55.39594, 10.38621]
"10084" = [56.16, 10.20]
```

Vægter does not geocode. A host with only a city in its inventory is counted on the status line, not placed.

| Marker | Host |
|---|---|
| `●` | Available, no problems above information |
| `▲` | A warning or average problem |
| `■` | A high or disaster problem; disaster pulses between `■` and `○` |
| `○` | Unreachable, disabled or in maintenance, with no problem |
| `2` to `9`, `+` | Several hosts in one cell; the worst decides the colour |

Shapes differ by status, so colour is never the only cue. ASCII mode uses `@ ^ # o`.

The Boards screen's `geomap` widget draws the same map at the widget's saved view, with the hosts it names.

## Palettes

| Palette | Use |
|---|---|
| `green` | Green phosphor, the default |
| `solarized-dark`, `solarized-light` | Lower glare, for glaucoma and similar needs |

Every palette is held by a test to the WCAG contrast targets against its own background: 4.5:1 for names, 3:1 for
roads, water and markers. Solarized light uses its orange for main roads and a darker green for healthy hosts,
because solarized's own yellow and green fall just below 3:1 on its light background. Park and land textures are
deliberately softer, so they stay behind roads and names.

## How it works

Everything is in `internal/mapterm`, which imports nothing from the rest of Vægter:

| File | Job |
|---|---|
| `mvt/` | Mapbox Vector Tile decoder, standard library only, fuzz-tested |
| `projection.go`, `tile.go`, `view.go` | Web Mercator, tile maths, the tiles a view needs, wrapping at the date line |
| `canvas.go`, `raster.go` | Braille canvas of 2×4 dots per cell, clipped lines, even-odd fills that keep holes |
| `style.go`, `frame.go` | What to draw from the OpenMapTiles schema, and the three drawing modes |
| `labels.go` | Names, ranked and placed without overlap, spelt in ASCII when needed |
| `palette.go` | Colours and the contrast calculation |
| `map.go` | Map state: pan, zoom, back, fit, layers, and the cached last frame |
| `markers.go` | Markers, grouping, blinking and pulsing, drawn over a copy of the frame |
| `source.go`, `loader.go`, `cache.go`, `disk.go` | Fetching, background loading, memory and disk caches |

Drawing never waits on the network. `Map.Render` draws from tiles already held and reports the rest. The UI asks the
loader for them, and each one is fetched in its own Bubble Tea command; the map draws again as each arrives. While a
tile loads, a cached one from up to three zooms lower stands in, enlarged. Tiles that scroll away are cancelled. A
failed tile waits 30 seconds before it is asked for again. The last frame is reused until the view, the settings or
the tiles change, and markers are drawn over a copy, so a blinking marker never redraws the map.

Zooms beyond the source's deepest level, 14 for OpenFreeMap, enlarge zoom 14 tiles up to zoom 19. Buildings show
from zoom 16, minor roads from 14, and paths and service roads from 16. Names of main roads show from zoom 14,
other street names from 15.5, and water names from 13.

## Caching and offline use

Tiles are cached in memory, about 100 to 200 city tiles (measured at 18,000 to 40,000 points each), and on disk, up to 200 MB with the least recently used removed
first. Files are private to your user. Disk tiles are grouped by OpenFreeMap's data build, which its tile URLs name,
so a new build never mixes with an old one. The last tile list is saved too, so Vægter starts and draws offline for
any area it has seen. A run with networking fully disabled drew a cached view identical to the online one.

## OpenFreeMap's terms

OpenFreeMap needs no key and sets no general rate limit. Vægter follows what its terms ask of applications:

- a User-Agent naming the application: `vaegter/<version> (+https://github.com/knightmare2600/vogter)`;
- at most four requests at once;
- only the tiles needed for what is on screen, never bulk downloads, which its terms forbid beyond offline areas of
  3,000 tiles or 50 MB;
- the credit "© OpenMapTiles © OpenStreetMap", always on the map's status line, and the last thing to go when the
  screen is narrow.

The service is provided as it is and may end without notice; the disk cache keeps seen areas working if it does.

## Attribution and licences

Map data © OpenStreetMap contributors, under the Open Database License: https://www.openstreetmap.org/copyright.
Vægter draws the data to its own design, which makes each screen a Produced Work under the ODbL, needing that
attribution. It is shown on the map, on `geomap` widgets, in the About box (`A`) and in `vaegter licences`. The tile
schema is OpenMapTiles; the tiles are served by OpenFreeMap. Vægter ships no map data; the cache holds only what you
have viewed.
