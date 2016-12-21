Application Operating Modes
===========================

Application has two basic modules: controller and scheduler. 
External IP Controller application may be run in one of the operating modes:
* [Simple](#simple-mode)
* [Daemon Set](#daemon-set)


Controller module manages IPs on its node (brings up, deletes, ensures that list
of IPs on node reflects the list of IPs required for services).
Controller(s) should be run on nodes that have external connectivity as
External IPs will be spawned on that nodes. Each node should have no more than
one controller at a time.

Scheduler module processes IP claims from services and distributes them among
controllers (i.e. nodes).
Scheduler(s) can be run on any nodes as they just schedule claims among the
controllers.


## Simple mode

External IP controller application will be run on one of the nodes. It should be
run on node that has external connectivity as Extrenal IPs will be spawned on
that node.

## Daemon Set


