#!/bin/bash
# Drive a binary through a fixed key script under tmux, against mockzbx,
# capturing each screen and the request log into a work directory.
#
#   tools/compare.sh <workdir> <name> <binary> <config> <mockzbx>
S=$1; name=$2; bin=$3; cfg=$4; mock=$5; sock=cmp-$name
$mock -log $S/cmp-$name.jsonl >/dev/null 2>&1 & MP=$!
sleep 0.4
tmux -L $sock new-session -d -x 150 -y 45 "env HOME=$S/home XDG_CONFIG_HOME=$S/home/.config COLORTERM=truecolor TERM=xterm-256color $bin -c $cfg"
snap() { tmux -L $sock capture-pane -p > $S/cmp-$name-$1.txt; }
keys() { tmux -L $sock send-keys "$@"; }
sleep 2.5; snap dash
keys 2; sleep 0.5; snap hosts
keys j; sleep 0.4; snap hosts2
keys s; sleep 0.3; keys / ; keys w e b; sleep 0.4; snap hostfilter; keys Escape; sleep 0.2
keys 3; sleep 0.5; snap problems
keys j; sleep 0.3; snap problems2
keys + +; sleep 0.3; snap sev; keys - -
keys h; sleep 0.3; snap hide; keys h
keys k; keys a; sleep 0.4; snap ack
keys l o o k; keys Enter; sleep 0.8; snap acked
keys '?'; sleep 0.4; snap help; keys Escape; sleep 0.3
keys 2; sleep 0.3; keys g j Enter; sleep 0.8; snap host
keys ]; sleep 0.6; snap host6h
keys ] ]; sleep 0.8; snap host7d
keys [ [ [; sleep 0.6
keys Tab; sleep 0.5; snap items
keys / ; keys c p u; sleep 0.4; snap itemfilter; keys Escape; sleep 0.2
keys Escape; sleep 0.3; snap back
sleep 0.3; keys q; sleep 0.8
tmux -L $sock kill-server 2>/dev/null
kill $MP
