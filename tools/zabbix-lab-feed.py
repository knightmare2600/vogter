# Seed the Zabbix lab with hosts and triggers, then feed it changing values so
# problems open and resolve. Run with python3 -I. Stop it with Ctrl-C.
# The credentials are the stock Zabbix defaults of a throwaway local container.
import json
import math
import random
import time
import urllib.request

URL = "http://127.0.0.1:18080/api_jsonrpc.php"
HOSTS = ["web-01", "web-02", "db-01", "cache-01"]
# Item key -> (name, units).
ITEMS = {
  "system.cpu.util": ("CPU utilisation", "%"),
  "vm.memory.util": ("Memory utilisation", "%"),
  "system.cpu.load[all,avg1]": ("Load average (1m)", ""),
  "system.uptime": ("Uptime", "uptime"),
  'net.if.in["eth0"]': ("Bits received", "bps"),
  'net.if.out["eth0"]': ("Bits sent", "bps"),
  "vfs.fs.dependent.size[/,pused]": ("FS [/]: Space: Used, in %", "%"),
  "vfs.fs.dependent.size[/var,pused]": ("FS [/var]: Space: Used, in %", "%"),
}
TRIGGERS = [
  ("High CPU on {h}", 3, "last(/{h}/system.cpu.util)>85"),
  ("Memory nearly full on {h}", 4, "last(/{h}/vm.memory.util)>90"),
  ("Disk / critically full on {h}", 5, "last(/{h}/vfs.fs.dependent.size[/,pused])>95"),
  ("Load is elevated on {h}", 2, "last(/{h}/system.cpu.load[all,avg1])>4"),
]

token = None


def call(method, params):
  headers = {"Content-Type": "application/json-rpc"}
  if token:
    headers["Authorization"] = "Bearer " + token
  body = json.dumps({"jsonrpc": "2.0", "method": method, "params": params, "id": 1}).encode()
  req = urllib.request.Request(URL, body, headers)
  with urllib.request.urlopen(req, timeout=60) as resp:
    reply = json.load(resp)
  if "error" in reply:
    raise SystemExit(f"{method}: {reply['error']}")
  return reply["result"]


def wait_for_api():
  for _ in range(120):
    try:
      return call("apiinfo.version", {})
    # The API answers with an error until the database schema is loaded.
    except (OSError, SystemExit):
      time.sleep(5)
  raise SystemExit("the Zabbix API did not start")


def seed():
  # The stock host points at 127.0.0.1; the agent is in its own container.
  host = call("host.get", {"filter": {"host": ["Zabbix server"]}, "selectInterfaces": ["interfaceid"]})[0]
  call("hostinterface.update", {
    "interfaceid": host["interfaces"][0]["interfaceid"], "useip": 0, "dns": "vl-agent", "ip": "",
  })
  group = call("hostgroup.create", {"name": "Vogter lab"})["groupids"][0]
  for name in HOSTS:
    hostid = call("host.create", {
      "host": name, "groups": [{"groupid": group}], "tags": [{"tag": "env", "value": "lab"}],
    })["hostids"][0]
    for key, (label, units) in ITEMS.items():
      vtype = 3 if key == "system.uptime" else 0
      call("item.create", {
        "hostid": hostid, "name": label, "key_": key, "type": 2, "value_type": vtype, "units": units,
      })
    for desc, prio, expr in TRIGGERS:
      call("trigger.create", {"description": desc.format(h=name), "priority": prio, "expression": expr.format(h=name)})
  # Discovery otherwise waits up to an hour.
  rules = call("discoveryrule.get", {"output": ["itemid"], "hostids": [host["hostid"]]})
  call("task.create", [{"type": 6, "request": {"itemid": r["itemid"]}} for r in rules])


def feed():
  items = call("item.get", {
    "output": ["itemid", "key_", "hostid"], "selectHosts": ["host"],
    "filter": {"host": HOSTS, "type": 2},
  })
  start = time.time()
  while True:
    now = time.time() - start
    batch = []
    for it in items:
      host = it["hosts"][0]["host"]
      seed_ = sum(map(ord, host)) % 7
      wave = math.sin(now / 90 + seed_)
      value = {
        "system.cpu.util": 50 + 42 * wave + random.uniform(-4, 4),
        "vm.memory.util": 70 + 24 * math.sin(now / 200 + seed_) + random.uniform(-1, 1),
        "system.cpu.load[all,avg1]": max(0, 2.5 + 2.5 * wave),
        "system.uptime": 86400 * (seed_ + 1) + int(now),
        'net.if.in["eth0"]': 4e6 * (1.2 + wave) + random.uniform(0, 5e5),
        'net.if.out["eth0"]': 2e6 * (1.2 - wave) + random.uniform(0, 5e5),
        "vfs.fs.dependent.size[/,pused]": 90 + 7 * math.sin(now / 300 + seed_),
        "vfs.fs.dependent.size[/var,pused]": 40 + seed_ * 5,
      }[it["key_"]]
      batch.append({"itemid": it["itemid"], "value": int(value) if it["key_"] == "system.uptime" else round(value, 2)})
    call("history.push", batch)
    time.sleep(10)


wait_for_api()
token = call("user.login", {"username": "Admin", "password": "zabbix"})
if not call("hostgroup.get", {"filter": {"name": ["Vogter lab"]}}):
  seed()
  time.sleep(20)
print("feeding the lab; Ctrl-C stops it", flush=True)
try:
  feed()
except KeyboardInterrupt:
  pass
