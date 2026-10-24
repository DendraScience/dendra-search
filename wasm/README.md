# Product WASM host

The Explore browser host. Unlike `examples/wasm` it builds no fixtures
and generates no data: the page fetches real shard bytes (the public
container, then signed member shards) and hands them in.

```bash
./wasm/build.sh                 # writes wasm/search.wasm + wasm_exec.js
./wasm/build.sh path/to/out.wasm
```

JS (after `Go` from `wasm_exec.js` has run the module, and
`globalThis.__searchOnReady` has fired):

```js
JSON.parse(searchLoadContainer(new Uint8Array(frameBytes)))
// -> {organizations: [orgId]} — ParseContainer + Open of every chunk.
//    A member shard already open for an org keeps precedence.

JSON.parse(searchLoadMemberShard(orgId, new Uint8Array(shardBytes)))
// -> {organization: orgId} — replaces that org's public chunk.

JSON.parse(searchGroupIDs(JSON.stringify({ begins_at: 0, ends_before: 0 })))
// -> {groups_by_org: {orgId: [groupId]}} — public access on public
//    chunks, member access on private shards. Zero bounds = open.

JSON.parse(searchReducePicker(JSON.stringify({
  orgs: [{id, name}], groups: [{id, name}],
  checked_orgs: [], checked_groups: [], groups_by_org: {}
})))
// -> {orgs: [{id, name, grey}], groups: [{id, name, grey}]}
```

Failures are `{"error":"..."}` and keep the shards already open. The
module never fetches and never gunzips: the page's `fetch` has already
stripped `Content-Encoding: gzip` from the container response.
