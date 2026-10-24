# Three-node Swarm qualification

`os/tests/qemu-swarm.py` boots three independent signed appliance images under
KVM, each with its own datastore and firmware state. A loopback-only multicast
Ethernet segment carries their static addresses. The test does not join or
modify a host Docker cluster. It needs approximately 12 GiB of VM memory.

The required checks are:

1. Three manager nodes form a healthy Raft quorum.
2. Global jobs receive secrets and configs on every node with the requested
   permissions and read-only mounts.
3. A client on one node resolves and reaches a service constrained to another
   node over an encrypted overlay. Packet captures must contain ESP and must
   not contain the application response in plaintext.
4. The published routing-mesh port works through every node, including nodes
   without a task. Scaling and drain move the desired tasks onto available nodes.
5. Abrupt leader VM termination permits a replacement leader, a new scheduling
   change and successful traffic through survivors. The stopped manager rejoins
   from its existing datastore.
6. Service update and rollback complete, including the expected configuration
   before and after rollback.
7. Losing two managers refuses scheduling changes while an existing task still
   serves traffic. Restarting the managers restores quorum and scheduling.
8. Managers are demoted while quorum exists, workers leave, and the final
   single-manager swarm is dissolved. Each operation has a bounded deadline;
   every engine must report inactive membership afterward.

The harness uses [Docker's manager-quorum rules](https://docs.docker.com/engine/swarm/admin_guide/),
[encrypted overlay support](https://docs.docker.com/engine/network/drivers/overlay/),
and [service/job, secret and config semantics](https://docs.docker.com/reference/cli/docker/service/create/).
Decommissioning follows [Docker's demote-before-leave procedure](https://docs.docker.com/reference/cli/docker/swarm/leave/).
Service removal is asynchronous. Before removing overlay networks or leaving
the swarm, the harness waits until every node reports that the removed
services' task containers are gone, with a bounded deadline.
An earlier stress run that force-removed a manager from a multi-manager cluster
returned `context deadline exceeded`. Passing the normal decommissioning test
does not qualify that forced-removal path; its failed evidence is retained.
After abrupt host loss, the service summary can include last-reported Running
states for orphaned tasks. Failover assertions therefore require current desired
task placements on live nodes and successful traffic, rather than trusting only
the aggregate replica count.

Run directly on a Linux KVM host with QEMU, OVMF and the Python test dependencies:
The script creates disposable VMs and needs sufficient storage for their disks.
Use a new output directory.

```
python3 os/tests/qemu-swarm.py --artifacts /path/to/images --output /path/to/new-test-directory
```

The result records both image hashes, each completed check and ESP packet count.
The three `overlay-only-*.pcap` files preserve the encrypted test phase before
plaintext ingress testing begins, with their SHA-256 values in the result.
Serial logs, captures and disposable VM disks remain in the test directory.
A failed or missing check leaves `passed` false. This qualifies the listed
behaviors using runc on three QEMU nodes; Kata is qualified separately. It does
not establish production-scale throughput,
physical switching compatibility, cross-datacenter latency or long-duration
failure tolerance.
