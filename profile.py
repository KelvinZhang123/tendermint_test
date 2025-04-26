# -*- coding: utf-8 -*-
import geni.portal as portal
import geni.rspec.pg as rspec

NUM_NODES = 4

pc = portal.Context()
request = rspec.Request()

lan = request.LAN('lan0')
lan.best_effort = False

for i in range(NUM_NODES):
    name = "node{}".format(i)
    node = request.RawPC(name)
    iface = node.addInterface("if{}".format(i))
    lan.addInterface(iface)

    cmd = """
#!/bin/bash
set -e

curl -L https://git.io/fFfOR | bash
source ~/.profile

TMHOME=/local/node{0}
mkdir -p $TMHOME
tendermint init --home $TMHOME

NODE_ID=$(tendermint show_node_id --home $TMHOME)
IP=$(hostname -I | awk '{{print $1}}')
echo ">>> {1} ID=$NODE_ID at $IP:26656 <<<"
""".format(i, name)

    node.addService(rspec.Execute(shell="bash", command=cmd))

pc.printRequestRSpec(request)
