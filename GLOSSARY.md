# BatStat

BatStat observes network connections and shows the processes associated with them.

## Language

**Connection snapshot**:
The set of network connections observed during a single refresh.

**Paused monitoring**:
The mode in which BatStat keeps the last displayed connection snapshot available for inspection and export.

**Snapshot diff**:
A comparison between two connection snapshots that shows which connections appeared and which vanished.

**Previous snapshot**:
The most recent successful connection snapshot displayed before the current one. After paused monitoring, the first new snapshot uses the last displayed snapshot as its previous snapshot.

**New connection**:
A connection observed in the current snapshot that was absent from the comparison baseline.

**Vanished connection**:
A connection observed in the comparison baseline that is absent from the current snapshot.

**Recently vanished connection**:
A vanished connection retained briefly in the live snapshot diff so it can be inspected after subsequent refreshes.

**Pinned snapshot**:
The complete connection snapshot held as a fixed baseline for before-and-after comparisons during a BatStat session.

**TCP listener**:
A TCP socket observed in the `LISTEN` state.

**Loopback binding**:
A listener bound to a local loopback address, such as `127.0.0.1` or `::1`.

**All-interface binding**:
A listener bound to the wildcard address for its address family, `0.0.0.0` for IPv4 or `::` for IPv6.

**Process group**:
The observed connections attributed to one process ID in the current connection snapshot.

**Unknown owner group**:
The connections in the current snapshot whose owning process IDs are unavailable. They can belong to several processes.
