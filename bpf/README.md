# Kernel chaos layer

`hyperion_chaos.bpf.c` provides CO-RE-compatible XDP ingress drops/partitions
and TC egress drops/corruption. Build and load it only inside an isolated Linux
network namespace.

Packet delay is intentionally delegated to a `netem` qdisc controlled by the
same userspace fault plan. A TC classifier cannot safely block while retaining
packets; `netem` is the kernel facility designed for delay and reordering.

The loader must always:

1. create a dedicated network namespace and veth pair;
2. attach XDP/`clsact` only inside that namespace;
3. update the policy map atomically;
4. detach programs and delete qdiscs in a deferred cleanup path.

Never attach the development policy to a host management interface.

