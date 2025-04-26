# profile.py
# -*- coding: utf-8 -*-
import geni.portal as portal
import geni.rspec.pg as rspec

NUM_NODES = 4

pc = portal.Context()
request = rspec.Request()

# Create a private LAN for all replicas
lan = request.LAN('lan0')
lan.best_effort = False

for i in range(NUM_NODES):
    # Name them node0, node1, ...
    node = request.RawPC(f'node{i}')
    iface = node.addInterface(f'if{i}')
    lan.addInterface(iface)

    # Bootstrap script: install Tendermint, init a home, show node ID
    node.addService(rspec.Execute(
        shell='bash',
        command=f"""#!/bin/bash
set -e

# 1) Quick-install Go & Tendermint
curl -L https://git.io/fFfOR | bash
source ~/.profile

# 2) Prepare a private TM home for this replica
TMHOME=/local/node{i}
mkdir -p $TMHOME

# 3) Initialize Tendermint (generates config & genesis)
tendermint init --home $TMHOME

# 4) Print out this node's ID and IP so you can assemble persistent_peers
NODE_ID=$(tendermint show_node_id --home $TMHOME)
IP=$(hostname -I | awk '{{print $1}}')
echo ">>> node{i} ID=$NODE_ID at $IP:26656 <<<"

# 5) (optional) start Tendermint with a placeholder peer list
#    Replace the PERSISTENT_PEERS below with the comma-separated list
#    you collect from the four nodes' echo lines above.
# tendermint node \
#   --home $TMHOME \
#   --proxy_app=kvstore \
#   --p2p.laddr tcp://0.0.0.0:26656 \
#   --p2p.persistent_peers="PERSISTENT_PEERS"
"""
    ))

portal.Context().printRequestRSpec(request)
