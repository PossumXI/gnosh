# Carried files

This file lists every file in this repository that was not written for it,
and (under "Words carried into `internal/prompt`") the wording that was
carried inside files that were:
the files brought over from the earlier gnosh, the program of the same name
that was a fork of Charmbracelet's Crush. PROVENANCE.md says how such files
are chosen ("How files are carried over"); this is the record of what was
brought, where each file came from, what the comparison with Charm's code
measured for it, the checksum of the copy that was staged, and what was
changed on the way in. It is an engineering record, not legal advice.

| what | value |
|---|---|
| the earlier gnosh | github.com/gnosh-ai/gnosh, `main` at `1c77d2bd58482c37dd858b24326a6e1829117904` |
| the fork point the scan used (its merge base with charmbracelet/crush `main`) | `52c75da8acbcd11dc269ae582f3ed7947ba3cff6` |
| brought over on | 13 November 2026, on branch `port/gnovm` |

The inventory, on the spec side, compared every file the earlier gnosh had
added with Charm's code and staged the ones that passed, byte for byte. The
porter, on the build side, read the staged files, the inventory's manifest
(words and numbers) and its scan table, and nothing else of the earlier tree.
Every carried file keeps the path it had.

## What was brought

<!-- carried-summary:begin (written by tools/audit/carried.py; do not edit by hand) -->

| what | files | bytes | of which |
|---|---:|---:|---|
| `GNOMEM.md`: mem's own documentation, at the repository root: a carried test reads it | 1 | 56,358 | 1 carried |
| `gno/r/gnosh/mem/`: the shared mem realm for gno.land | 6 | 68,058 | 6 carried |
| `internal/gnochain/`: the gno.land client | 9 | 60,682 | 8 carried, 1 carried with changes |
| `internal/gnovm/internal/syncstdlibs/`: the tool that copies gno's standard library in | 1 | 4,074 | 1 carried |
| `internal/gnovm/klashlibs/fmt/`: the VMs' fmt library | 1 | 5,916 | 1 carried |
| `internal/gnovm/klashlibs/klash/gnomem/v2/`: gnomem v2, mem's realm in the local VM | 13 | 176,458 | 13 carried |
| `internal/gnovm/klashlibs/klash/bptree/v0/`: gno's B+tree package, with its LICENSE.md and SOURCE | 5 | 64,230 | 5 third-party |
| `internal/gnovm/stdlibs/`: gno's standard library, with its LICENSE.md, LICENSE-GO and VERSION | 163 | 1,499,963 | 163 third-party |
| `internal/gnovm/testdata/`: mem's scenarios, goldens and upgrade fixtures | 91 | 360,974 | 91 carried |
| `internal/gnovm/`: the GnoVM machines and mem: the package's own files, DESIGN.md and LICENSE.md | 80 | 934,405 | 72 carried, 8 carried with changes |
| `internal/swarm/cmd/swarmdemo/`: the swarm demo | 3 | 41,115 | 2 carried, 1 carried with changes |
| **all** | **373** | **3,272,233** | 195 carried (1,468,799 bytes), 10 carried with changes (239,241 bytes), 168 third-party (1,564,193 bytes) |

<!-- carried-summary:end -->

`GNOMEM.md` was not on the list of what to bring. It is here because
`internal/gnovm`'s `TestDocMemKeepsWhatAnEarlierBuildKept` reads it from the
repository root, to check that the words it quotes are the words the code
says. It was staged, and is the earlier gnosh's own.

Seven more staged files came after these, and are not in the table above:
`internal/gnovm/tools/`, the agent's `gnovm_*` tools and their tests. They
were written against the earlier harness's tool interface, so they could not
come over with their import paths changed and nothing more. "The tools,
carried with larger changes" below is their record.

Nothing else that was staged was brought. In particular:

- `internal/testenv/` (2 staged files) was offered in case the carried tests
  used it. None does.

## The table

One row per carried file.

- **kind**: *carried* is the earlier gnosh's own work, here byte for byte.
  *carried with changes* is the same with the edits the last column names,
  and nothing else. *third-party* is gno's, under gno's licence, byte for
  byte: the inventory found each such file identical to the same file of
  github.com/gnolang/gno v1.5.1-0.20260928125126-5cdbc25fcde0 (the standard
  library under `gnovm/stdlibs`, the B+tree under
  `examples/gno.land/p/nt/bptree/v0`), and the licence, notice, VERSION and
  SOURCE files beside them came with them.
- **bytes** and **SHA-256** are of the staged original.
- **line share** is the share of the file's distinct significant lines that
  also stand somewhere in Charm's code, counting all of Crush's history
  (the scan's `ext_sigd_pct`). **longest run** is the longest run of
  consecutive lines the file shares with one Charm file (the scan's
  `ext_srun_longest`). Both are the inventory's numbers, from its
  `scan.tsv`; the inventory's manifest says how they are measured and what
  they mean. Binary test data has neither.
- The one run in the table, 23 lines in `internal/gnovm/stdlibs/LICENSE-GO`,
  is the text of Go's BSD licence, which Crush's history also holds.

The table is written and checked by `tools/audit/carried.py`:

    tools/audit/carried.py check

hashes every file below as it stands in the tree, with the listed edits
undone, and compares it with the checksum in its row; it also fails if the
carried directories hold a file that is neither in the table nor listed
under "Files written fresh". The files of `internal/gnovm/tools/` are held
to their own section instead, in the ways that section gives.

<!-- carried-table:begin (written by tools/audit/carried.py; do not edit by hand) -->

| new path | old path | kind | bytes | line share | longest run | SHA-256 of the staged original | what was changed |
|---|---|---|---:|---:|---:|---|---|
| `GNOMEM.md` | `GNOMEM.md` | carried | 56,358 | 0.0% | 0 | `ee7d139f66864f99fe2aadfe569d530b2a3ae22a5f7aeab87a4a6b3194ac623d` | nothing |
| `gno/r/gnosh/mem/calls.gno` | `gno/r/gnosh/mem/calls.gno` | carried | 12,579 | 0.0% | 0 | `29e951e729f3e1bdf644732db97b018f41069c5044f523efaed0bbd4e4e3f5c5` | nothing |
| `gno/r/gnosh/mem/gnomod.toml` | `gno/r/gnosh/mem/gnomod.toml` | carried | 79 | 0.0% | 0 | `31808b66f4c9b6df49ab9bd47a98b639931d64fe7a34bc4c81c2a0d209b5e34e` | nothing |
| `gno/r/gnosh/mem/mem.gno` | `gno/r/gnosh/mem/mem.gno` | carried | 12,122 | 0.0% | 0 | `36d29da66f211881dab690f492b32ef5e1334cb130246fb09e324df2c7068e14` | nothing |
| `gno/r/gnosh/mem/mem_test.gno` | `gno/r/gnosh/mem/mem_test.gno` | carried | 23,233 | 0.0% | 0 | `f53a0e5abb6b58971264829e9305a5c112ff8b7ac3408eebb1c380a530d56f30` | nothing |
| `gno/r/gnosh/mem/ready.gno` | `gno/r/gnosh/mem/ready.gno` | carried | 16,350 | 0.0% | 0 | `fd414546d0a9fcfce31cc8cb5166230af93b6fde9aba1490f4beaf0663d9e148` | nothing |
| `gno/r/gnosh/mem/views.gno` | `gno/r/gnosh/mem/views.gno` | carried | 3,695 | 0.0% | 0 | `1da2af1103437e92d7205e0f69f4f94d4c2882ab5210fce30424c6561b341808` | nothing |
| `internal/gnochain/client.go` | `internal/gnochain/client.go` | carried | 12,917 | 0.6% | 0 | `ba1bb673588af5342bb24f2bfec641c8763102e38b10817f78f47946515dcf44` | nothing |
| `internal/gnochain/client_test.go` | `internal/gnochain/client_test.go` | carried | 13,414 | 1.1% | 0 | `3d7045d6567233ae86c0b0f9b844ec85f168924a9d1c8384842d1047bf17ac8b` | nothing |
| `internal/gnochain/follow.go` | `internal/gnochain/follow.go` | carried | 5,054 | 0.0% | 0 | `4b98f7ceece8a41d6ca05bd9ba54f9460caee4dbcb27a14426764ab39d04e8db` | nothing |
| `internal/gnochain/keysigner.go` | `internal/gnochain/keysigner.go` | carried | 2,197 | 0.0% | 0 | `e6ea1d2c24cdbca2b7c98a1772764b736edd6d068a640163f9e2bc4c609e3b1c` | nothing |
| `internal/gnochain/msg.go` | `internal/gnochain/msg.go` | carried with changes | 1,981 | 0.0% | 0 | `9d6da1bd5793989b4678d0c353406a4463b3d10b5cd15816397054c0d7ad4464` | import path, 1 line: the package's own path as it is registered with amino, which refuses a type whose package path differs |
| `internal/gnochain/rpc.go` | `internal/gnochain/rpc.go` | carried | 6,267 | 2.7% | 0 | `aca843cdb6e264f113057bdb106e79d64e9de8709c80f6741166ce203c47052e` | nothing |
| `internal/gnochain/sender.go` | `internal/gnochain/sender.go` | carried | 6,052 | 0.0% | 0 | `84151ef27fe1511c3d4e260914bc69439da6204d0de03909bc248173b5566e69` | nothing |
| `internal/gnochain/signer.go` | `internal/gnochain/signer.go` | carried | 6,070 | 0.0% | 0 | `5dd9af83f96aaadd92297e708db30545790b6c6283a0dba151fb78e14f741861` | nothing |
| `internal/gnochain/swarm_test.go` | `internal/gnochain/swarm_test.go` | carried | 6,730 | 3.8% | 0 | `4e44d17427202df30d1005b797e6bd352f8b031bcd7c68afc72ee04fbd8bd8ef` | nothing |
| `internal/gnovm/DESIGN.md` | `internal/gnovm/DESIGN.md` | carried | 67,226 | 0.0% | 0 | `b1afaacb1d96ce3a734c9e504a24192a4aacda27f49776016a2c447fc8ebd4a4` | nothing |
| `internal/gnovm/LICENSE.md` | `internal/gnovm/LICENSE.md` | carried | 44,843 | 0.0% | 0 | `f8cc447c85ad90c04b976c8c3f2c4a8062c0cdabe826003a89296fd2f61c548f` | nothing |
| `internal/gnovm/base.go` | `internal/gnovm/base.go` | carried | 6,250 | 0.0% | 0 | `157faf3eacf435f88d22ddb104ed3527fe057edf7e91ba3670912f8edb030986` | nothing |
| `internal/gnovm/budget.go` | `internal/gnovm/budget.go` | carried | 5,727 | 0.0% | 0 | `ecfef5ed402a750c6c84a6ecdd0466e02ac2f3e37a8c82127670e1a4a83f2a61` | nothing |
| `internal/gnovm/client.go` | `internal/gnovm/client.go` | carried with changes | 24,348 | 0.7% | 0 | `c552bd0ae975c8f2f97190cc865a59da36d9dcec6ca6909144e3bcd475f4649f` | import path, 1 line: the earlier harness's `internal/lock` is now `internal/filelock`, imported under the name `lock` so no call changed |
| `internal/gnovm/client_unix.go` | `internal/gnovm/client_unix.go` | carried | 784 | 9.1% | 0 | `6d756b6fd22b27c5702073d709d0abd26143222b38ec0591d2972ea49bcd7965` | nothing |
| `internal/gnovm/client_windows.go` | `internal/gnovm/client_windows.go` | carried | 637 | 0.0% | 0 | `8e2bf614ca2edbe87ced8cbe2c7a3c31a3cf0d98e6fd9ee3ef0ac46860d0ba75` | nothing |
| `internal/gnovm/differential_test.go` | `internal/gnovm/differential_test.go` | carried | 4,318 | 1.7% | 0 | `7dfe41faa8f7580b091dab3f6dcd4228d6980c113b9609aaad06fe35aacd7c9b` | nothing |
| `internal/gnovm/errors.go` | `internal/gnovm/errors.go` | carried | 4,647 | 1.4% | 0 | `b66e5c80dfdb541986e2d69c5f3ff6a2f531972318fc0cc81ac9ddf46e62aa08` | nothing |
| `internal/gnovm/fmt_test.go` | `internal/gnovm/fmt_test.go` | carried | 5,454 | 0.0% | 0 | `6d42f55f6bb61133bf5a7b5c2357b8c0bee4ab3cfa751e859ef06156cc53564d` | nothing |
| `internal/gnovm/fmtnative.go` | `internal/gnovm/fmtnative.go` | carried | 13,757 | 1.2% | 0 | `6b56a498422c2b4367756cd92130716131e27f3dc17f642185947d4a84f75460` | nothing |
| `internal/gnovm/frames_test.go` | `internal/gnovm/frames_test.go` | carried | 12,001 | 0.0% | 0 | `9a6b81826acde4d50ae16d7daf99cfa0bc0bac9adf1b5606ec3e90cbfa06ed8e` | nothing |
| `internal/gnovm/gnomem2_test.go` | `internal/gnovm/gnomem2_test.go` | carried | 9,962 | 0.0% | 0 | `26f45dd1ae0d2582d8dea4ecd0dd059025cd928ddf275e9ba090f785c9e610ca` | nothing |
| `internal/gnovm/hints.go` | `internal/gnovm/hints.go` | carried | 8,322 | 0.0% | 0 | `cc1d27f0dac6dfcd00d637394927c1d02e59d3880cfab95c80aa5304ec50a80d` | nothing |
| `internal/gnovm/hints_test.go` | `internal/gnovm/hints_test.go` | carried | 3,358 | 0.0% | 0 | `dd4ab443c8b5e1c6a86d040f2b30042f6ca7e1a747aedda86140e7e1a73527ef` | nothing |
| `internal/gnovm/host.go` | `internal/gnovm/host.go` | carried with changes | 48,932 | 0.3% | 0 | `3d62806231c07ed69fe0049e133c43669f45763a94b41fd465b56f51888925de` | import path, 1 line: the earlier harness's `internal/lock` is now `internal/filelock`, imported under the name `lock` so no call changed |
| `internal/gnovm/host_test.go` | `internal/gnovm/host_test.go` | carried | 10,903 | 0.7% | 0 | `4837b4b98c47d166a1418625e68a8f3e90b90c0efee67a0aae9f6d877e551f05` | nothing |
| `internal/gnovm/hostmem.go` | `internal/gnovm/hostmem.go` | carried | 4,050 | 0.0% | 0 | `a850ec55ce23f9185e94966f5cd2ee43b78ec5cc1d3b92b40f330e6390718079` | nothing |
| `internal/gnovm/hostmem_darwin.go` | `internal/gnovm/hostmem_darwin.go` | carried | 1,226 | 0.0% | 0 | `51d5e6c20aceb6f849a969f4d3323650e6f486859c501405945ac7192b07a616` | nothing |
| `internal/gnovm/hostmem_linux.go` | `internal/gnovm/hostmem_linux.go` | carried | 1,337 | 0.0% | 0 | `2cbc5eadd99d2faff91300958a05832b94f96cd786970a82cbaf61e3585003c1` | nothing |
| `internal/gnovm/hostmem_other.go` | `internal/gnovm/hostmem_other.go` | carried | 769 | 0.0% | 0 | `0567137057f3be31efb22f7890e32627dd66544d5483a4bc670dbafe1553d62b` | nothing |
| `internal/gnovm/hostmem_test.go` | `internal/gnovm/hostmem_test.go` | carried | 3,251 | 1.9% | 0 | `a4de679fcb4fe10041f90b978d962fd451553ef8081493eb245c12f36a1d3703` | nothing |
| `internal/gnovm/internal/syncstdlibs/main.go` | `internal/gnovm/internal/syncstdlibs/main.go` | carried | 4,074 | 0.0% | 0 | `9e48311c7b15d2c13c5f173d73c1817ccef1a1cf6941e9f04495d4ffe93fff78` | nothing |
| `internal/gnovm/journal.go` | `internal/gnovm/journal.go` | carried | 14,552 | 1.6% | 0 | `084b0fe1bbbbda98cdbaf90bc73a9b9bdf203a5d06ab2734965a0d8b9d48e45c` | nothing |
| `internal/gnovm/journal_test.go` | `internal/gnovm/journal_test.go` | carried | 9,662 | 0.9% | 0 | `59f775bdf61b8f204d07a0e4b6dc3a2200fb37d2fc8097df0b1b5090874446a1` | nothing |
| `internal/gnovm/klashlibs/fmt/fmt.gno` | `internal/gnovm/klashlibs/fmt/fmt.gno` | carried | 5,916 | 0.0% | 0 | `ac3c3523659e9e4b2af8b24208bc75f7ec6df01ee4279e52b88d69e64bb8aea9` | nothing |
| `internal/gnovm/klashlibs/klash/bptree/v0/LICENSE.md` | `internal/gnovm/klashlibs/klash/bptree/v0/LICENSE.md` | third-party | 38,423 | 0.0% | 0 | `3153999d8faec36dd083e050f6e1ba5d76daa3dca7858a420d852a5043e7f80b` | nothing |
| `internal/gnovm/klashlibs/klash/bptree/v0/SOURCE` | `internal/gnovm/klashlibs/klash/bptree/v0/SOURCE` | third-party | 173 | 0.0% | 0 | `f1461a174f93e533b29693bd9e98fe8761cd8bdb1275e3273468fa57fb9b5bf7` | nothing |
| `internal/gnovm/klashlibs/klash/bptree/v0/doc.gno` | `internal/gnovm/klashlibs/klash/bptree/v0/doc.gno` | third-party | 632 | 0.0% | 0 | `33fbd751b602fd9868747364cb4ea6b175707827c0c3df149e17a6460ed02327` | nothing |
| `internal/gnovm/klashlibs/klash/bptree/v0/node.gno` | `internal/gnovm/klashlibs/klash/bptree/v0/node.gno` | third-party | 4,071 | 0.0% | 0 | `82de343f997aa5ecfc78bff785599193cead0ebc93b666b812491f45c2bd1310` | nothing |
| `internal/gnovm/klashlibs/klash/bptree/v0/tree.gno` | `internal/gnovm/klashlibs/klash/bptree/v0/tree.gno` | third-party | 20,931 | 0.0% | 0 | `dde357da5d6d12b342b1bed06a897eb1f9a92b9ab96c0b40e0d97952b273a948` | nothing |
| `internal/gnovm/klashlibs/klash/gnomem/v2/apply.gno` | `internal/gnovm/klashlibs/klash/gnomem/v2/apply.gno` | carried | 16,558 | 0.0% | 0 | `4ac99cc508b0d70d9b3035922258da5f0ad46277e7474b79a0ef72fc8220d2c9` | nothing |
| `internal/gnovm/klashlibs/klash/gnomem/v2/check.gno` | `internal/gnovm/klashlibs/klash/gnomem/v2/check.gno` | carried | 5,083 | 0.0% | 0 | `0736d3657a0ad70850635ae8bdb888ee663dba0e382c4f4fb1c1a1a74150abc0` | nothing |
| `internal/gnovm/klashlibs/klash/gnomem/v2/frames.gno` | `internal/gnovm/klashlibs/klash/gnomem/v2/frames.gno` | carried | 50,780 | 0.3% | 0 | `93da0b836e5ecb42f6f39ce0e825875e7a2604b5ca3c7af2e165438cfd4c59f5` | nothing |
| `internal/gnovm/klashlibs/klash/gnomem/v2/gnomem.gno` | `internal/gnovm/klashlibs/klash/gnomem/v2/gnomem.gno` | carried | 24,838 | 0.0% | 0 | `a449e1037e0d03b76e30c60f9a36eac4727fa9edd6e241c9aac0e5e1da964100` | nothing |
| `internal/gnovm/klashlibs/klash/gnomem/v2/json.gno` | `internal/gnovm/klashlibs/klash/gnomem/v2/json.gno` | carried | 7,415 | 0.0% | 0 | `7f7d1b32d173995b067fe41c471a435d81e248f122f2fb90bd3fbffbbdfc54e2` | nothing |
| `internal/gnovm/klashlibs/klash/gnomem/v2/names.gno` | `internal/gnovm/klashlibs/klash/gnomem/v2/names.gno` | carried | 14,174 | 1.1% | 0 | `9014e0373520ef72253e76f84e3b937b47d5726b445a43f265610a8c2cac8b4b` | nothing |
| `internal/gnovm/klashlibs/klash/gnomem/v2/natives.gno` | `internal/gnovm/klashlibs/klash/gnomem/v2/natives.gno` | carried | 4,945 | 0.0% | 0 | `a46627d4fdece5d8df24da3585b2af6e6363f6fe1b2f03028eae022963d48940` | nothing |
| `internal/gnovm/klashlibs/klash/gnomem/v2/next.gno` | `internal/gnovm/klashlibs/klash/gnomem/v2/next.gno` | carried | 6,089 | 0.0% | 0 | `6930783ed7f16e4e40e3a617ca46a805b1128fd935206ac01470589be6c43ed0` | nothing |
| `internal/gnovm/klashlibs/klash/gnomem/v2/peers.gno` | `internal/gnovm/klashlibs/klash/gnomem/v2/peers.gno` | carried | 7,818 | 0.0% | 0 | `f667fd758b2642cc80520f67afcd2a9949192ed031e50ad9f574d932e94c0e34` | nothing |
| `internal/gnovm/klashlibs/klash/gnomem/v2/show.gno` | `internal/gnovm/klashlibs/klash/gnomem/v2/show.gno` | carried | 19,238 | 0.0% | 0 | `a6efcca16010d1116ca9102472d595af9e989f903d6af52e3bea2239d36dfbaa` | nothing |
| `internal/gnovm/klashlibs/klash/gnomem/v2/steps.gno` | `internal/gnovm/klashlibs/klash/gnomem/v2/steps.gno` | carried | 2,070 | 3.6% | 0 | `2767fc6c8a32fcd9f3af4616291d400163b9813b73615409645cc1040c9e9806` | nothing |
| `internal/gnovm/klashlibs/klash/gnomem/v2/types.gno` | `internal/gnovm/klashlibs/klash/gnomem/v2/types.gno` | carried | 7,134 | 0.0% | 0 | `00bd179145c818075531945a3ebe1ef2fe70ae09cdfcb17d61695f45f2d5184e` | nothing |
| `internal/gnovm/klashlibs/klash/gnomem/v2/wait.gno` | `internal/gnovm/klashlibs/klash/gnomem/v2/wait.gno` | carried | 10,316 | 0.0% | 0 | `87f0c52f2b65437c08a4545b4cd0a1f05adee7558c0194d328687561b1c9527b` | nothing |
| `internal/gnovm/lazy.go` | `internal/gnovm/lazy.go` | carried | 1,942 | 0.0% | 0 | `d6896d9fcc1cf04e15ddd319fddf62a8b9b0257b569b3221fe1d5282d0c45503` | nothing |
| `internal/gnovm/mem_docs_test.go` | `internal/gnovm/mem_docs_test.go` | carried | 2,812 | 0.0% | 0 | `45d79e1802f765acb7f9ec1d2568fad9ceb4fbe6e69cfb774738ea7691b61025` | nothing |
| `internal/gnovm/memapi.go` | `internal/gnovm/memapi.go` | carried | 7,750 | 0.0% | 0 | `3f91478454af77065cf15f7b82e02c790973477c591461463ebfa67b4926232a` | nothing |
| `internal/gnovm/memblock.go` | `internal/gnovm/memblock.go` | carried | 10,384 | 0.7% | 0 | `c3d8f0564a09570c37996417d1fc773516633f59e4c2507d95d29100db3a7ebf` | nothing |
| `internal/gnovm/memcmd.go` | `internal/gnovm/memcmd.go` | carried | 15,943 | 0.0% | 0 | `ae685e8aebfab81575ecbd3e378b2ba9f33b798365bfa0b50b630dd8ab0b174a` | nothing |
| `internal/gnovm/memcmd_test.go` | `internal/gnovm/memcmd_test.go` | carried | 10,919 | 0.0% | 0 | `9d7b4ed4a26f163432314f24a4cf41116e026f914587be87c250d84ebb4dd2c7` | nothing |
| `internal/gnovm/memerasure.go` | `internal/gnovm/memerasure.go` | carried | 3,419 | 0.0% | 0 | `8b439b4a6d2e4c14689e9969bff1f3bf355bf03630403e15944b132bde2e55a1` | nothing |
| `internal/gnovm/memerasure_scale_test.go` | `internal/gnovm/memerasure_scale_test.go` | carried | 2,843 | 0.0% | 0 | `ee27af59664cbc89c98c0141a53f19bacd72407d157731a1fdcfc85dd0f910b7` | nothing |
| `internal/gnovm/memerasure_test.go` | `internal/gnovm/memerasure_test.go` | carried | 3,273 | 0.0% | 0 | `15f01b5458d7cb24e273425e8765cb88f2e08bef4310342f5db89820e2e2fa3e` | nothing |
| `internal/gnovm/memframes_test.go` | `internal/gnovm/memframes_test.go` | carried | 9,316 | 0.0% | 0 | `a223a3ec7e6f03d2a5ac9fdfa5d37a147285f4f393e01378af6af4bcfcaf58d5` | nothing |
| `internal/gnovm/memgoal.go` | `internal/gnovm/memgoal.go` | carried | 19,953 | 0.8% | 0 | `f515661daf4e47a3865908f66893869d55d7f09d3f64938a99193651ba609faf` | nothing |
| `internal/gnovm/memgoal_test.go` | `internal/gnovm/memgoal_test.go` | carried | 11,252 | 0.0% | 0 | `81c7907f02d5c81cc820e75318d796797f53e0be0550a2fc4fd1b5f50cb94c3f` | nothing |
| `internal/gnovm/memhost.go` | `internal/gnovm/memhost.go` | carried with changes | 38,983 | 1.0% | 0 | `00869487b2d50e2511ad24be82977a140c189de3bc726d02b480eb9715f965bd` | import path, 1 line: the earlier harness's `internal/lock` is now `internal/filelock`, imported under the name `lock` so no call changed |
| `internal/gnovm/memhost_test.go` | `internal/gnovm/memhost_test.go` | carried with changes | 57,721 | 1.3% | 0 | `128a21c2644d2bdd05253e6a02930a1b0cc56d604851bf5ecc155feecfb9ad18` | import path, 1 line: the earlier harness's `internal/lock` is now `internal/filelock`, imported under the name `lock` so no call changed |
| `internal/gnovm/memindex.go` | `internal/gnovm/memindex.go` | carried | 12,540 | 0.7% | 0 | `536d3bbe80a2c5e8646269de97cde73e1df56c09870dce5de486e416da66eca4` | nothing |
| `internal/gnovm/memindex_test.go` | `internal/gnovm/memindex_test.go` | carried | 4,355 | 1.8% | 0 | `63ca14a539379154ca6b8f585852c82ecaa28e2bc18d55343d978ae24dc13b58` | nothing |
| `internal/gnovm/meminput.go` | `internal/gnovm/meminput.go` | carried | 9,024 | 0.0% | 0 | `5ec0a27a766f9d0883bbc270c9af7ac65233bfaefee3b65022c8d21b03ce7e07` | nothing |
| `internal/gnovm/memjournal.go` | `internal/gnovm/memjournal.go` | carried | 13,577 | 0.6% | 0 | `bb1bbac75af80e154fc7b453cc539aca1b12d459cf76c5d475b23bc6c639f768` | nothing |
| `internal/gnovm/memnative.go` | `internal/gnovm/memnative.go` | carried | 10,422 | 0.8% | 0 | `ebe0e22f138b101cfc5dc627e99bdb8e3a472cd1a4a15139b8b4c36d8578c1e9` | nothing |
| `internal/gnovm/mempeers.go` | `internal/gnovm/mempeers.go` | carried | 9,483 | 1.5% | 0 | `22fd14e257950fc236eeb632d8e55b8001a65e3e572cca74f6fd668f9c0a6ca8` | nothing |
| `internal/gnovm/mempeers_host_test.go` | `internal/gnovm/mempeers_host_test.go` | carried with changes | 11,752 | 0.6% | 0 | `cec1ee94f77e10d33395a8c4a431d29455736b0bf8f1fb9f721944fb5ec8cfd8` | import path, 1 line: the earlier harness's `internal/lock` is now `internal/filelock`, imported under the name `lock` so no call changed |
| `internal/gnovm/mempeers_test.go` | `internal/gnovm/mempeers_test.go` | carried | 16,547 | 0.0% | 0 | `870c298ebbe1aceac6c9823ac26fc09b048d88f0c7cc7ac3c00a0fd5d4f08629` | nothing |
| `internal/gnovm/mempeers_unix.go` | `internal/gnovm/mempeers_unix.go` | carried | 3,638 | 0.0% | 0 | `9268396c47d29aca4356c186ce7a6bf8e5e43d6ce3750039ea0357ef8c620a6f` | nothing |
| `internal/gnovm/mempeers_windows.go` | `internal/gnovm/mempeers_windows.go` | carried | 1,094 | 0.0% | 0 | `15750e3186549ed1a2f1c6364d6966851e50e2ab06c88a02f2c12fe14facc343` | nothing |
| `internal/gnovm/memqa_test.go` | `internal/gnovm/memqa_test.go` | carried | 19,942 | 0.0% | 0 | `a78b3df229ca3d12b260771534dbe7a6e3b7a0e0aae6fb61672e9570aba638a5` | nothing |
| `internal/gnovm/memrealm.go` | `internal/gnovm/memrealm.go` | carried | 23,587 | 0.0% | 0 | `a0509d1aa326a8996789da9230d12f8d365dcc60cffb35abfe4c1c4abbe9aee0` | nothing |
| `internal/gnovm/memrealm_test.go` | `internal/gnovm/memrealm_test.go` | carried | 5,137 | 0.0% | 0 | `c50aa4a9df1a4ddff7eda1c7d24f8fef0b34123a91f5bc87a4c78ad5ddcd82f6` | nothing |
| `internal/gnovm/memrebuild_test.go` | `internal/gnovm/memrebuild_test.go` | carried | 6,095 | 0.0% | 0 | `ffb917640f7dd129dfbec4da5061257144c3f9d98076a514577e11d5bcaff62f` | nothing |
| `internal/gnovm/memrecall.go` | `internal/gnovm/memrecall.go` | carried | 5,659 | 0.0% | 0 | `3976411a828ad8011859007253322d1773f180e8ee8a296587c450b8fdc40a48` | nothing |
| `internal/gnovm/memrecall_live_test.go` | `internal/gnovm/memrecall_live_test.go` | carried | 7,809 | 0.9% | 0 | `155afa0b029abe4601d374362e28c13e9bf374ff9887ed9caa64ff93d5e6f0f4` | nothing |
| `internal/gnovm/memscale_test.go` | `internal/gnovm/memscale_test.go` | carried | 4,698 | 0.0% | 0 | `3a52eb37660b1648155f4cc19229e897e77b0c5b305ee704ba79c946639866e7` | nothing |
| `internal/gnovm/memvectors.go` | `internal/gnovm/memvectors.go` | carried | 28,331 | 1.1% | 0 | `38ba0bfb42b208d2bbccd65f679cd1883150c7e0a14da70afb03043f4cd28d7f` | nothing |
| `internal/gnovm/memvectors_test.go` | `internal/gnovm/memvectors_test.go` | carried | 25,909 | 0.6% | 0 | `2164e74b30bfa8a0c32aac4535f7a94031b6d75ae183cdcc51b6b215c387d92d` | nothing |
| `internal/gnovm/memwait.go` | `internal/gnovm/memwait.go` | carried | 6,553 | 1.2% | 0 | `2169791ddbb7500ff4389deb0066ea5e771d9fc0aa5c9e760b6df86a2794b3c1` | nothing |
| `internal/gnovm/memwait_test.go` | `internal/gnovm/memwait_test.go` | carried | 7,313 | 0.0% | 0 | `7bf93962fe9149491cc8c9fd622bf9e9a00da6f3ee5e4245eee7922ac0ed12d7` | nothing |
| `internal/gnovm/memwait_unix.go` | `internal/gnovm/memwait_unix.go` | carried | 4,396 | 1.9% | 0 | `ae6ef18ecd3a2ec9630603a7a7a5b2d95561bc40c401dd765805de21316142b3` | nothing |
| `internal/gnovm/memwait_windows.go` | `internal/gnovm/memwait_windows.go` | carried | 826 | 0.0% | 0 | `87f7175d9257d3913e1a2682516dc9e8325f288acee435c850e3e72d5384ed9b` | nothing |
| `internal/gnovm/meter.go` | `internal/gnovm/meter.go` | carried | 4,631 | 0.0% | 0 | `4e12f31e7de7f43035bf858a58ed050dcff0ac9bb5b3e2edb50df3fc3449e2f1` | nothing |
| `internal/gnovm/meter_test.go` | `internal/gnovm/meter_test.go` | carried | 4,564 | 0.0% | 0 | `7f9a8a6c2b3bc0cd6ddd28115e0292ace45ee23da71e6a05dfa0317dd30051f4` | nothing |
| `internal/gnovm/normalize.go` | `internal/gnovm/normalize.go` | carried | 23,598 | 0.3% | 0 | `beceaecba0c6237d2d9ac3af6db6188d7e8e1eb4bd34a52ff3a77c3d6248e749` | nothing |
| `internal/gnovm/peers_demo_test.go` | `internal/gnovm/peers_demo_test.go` | carried | 21,354 | 1.5% | 0 | `2dc04fc4698815c5b6c103b279e08b66a6b4800d0435958e66eb505cc38c19c7` | nothing |
| `internal/gnovm/peers_swarm_test.go` | `internal/gnovm/peers_swarm_test.go` | carried | 5,463 | 3.9% | 0 | `235e0123a0556944bb6af9718039301667cda69b5b5908ac0a2bb22f4ade0aa5` | nothing |
| `internal/gnovm/realworker_test.go` | `internal/gnovm/realworker_test.go` | carried with changes | 2,963 | 0.0% | 0 | `77fafa7a232572b5d6f1ea25795b0e3363a8eb62d7e3994606344f210897a960` | build instruction in a comment, 1 line: the linker flag names `internal/buildinfo.BuildID` under the new module path |
| `internal/gnovm/round3_test.go` | `internal/gnovm/round3_test.go` | carried | 8,442 | 1.7% | 0 | `43fea37b919bec3b5e30005a63ea73fd82985e23c8ca7320631ae6fbdc6ec8e6` | nothing |
| `internal/gnovm/round4_test.go` | `internal/gnovm/round4_test.go` | carried | 16,834 | 0.0% | 0 | `3444d78799317a174d1df50dbbf074b70b8c329ed75d56778999c23670f934ee` | nothing |
| `internal/gnovm/scenario_test.go` | `internal/gnovm/scenario_test.go` | carried | 38,975 | 2.2% | 0 | `dd3fb057671021a792873b8f6fa230876db1eec26891f3e152456d0c1330e825` | nothing |
| `internal/gnovm/stdlibs.go` | `internal/gnovm/stdlibs.go` | carried | 6,906 | 1.0% | 0 | `69c035bd62882bafb068947b865640a0ede6337a661c99c5841d212e19a0f9bf` | nothing |
| `internal/gnovm/stdlibs/LICENSE-GO` | `internal/gnovm/stdlibs/LICENSE-GO` | third-party | 1,453 | 100.0% | 23 | `911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad` | nothing |
| `internal/gnovm/stdlibs/LICENSE.md` | `internal/gnovm/stdlibs/LICENSE.md` | third-party | 38,276 | 0.0% | 0 | `e4c8727c74a30d51fc109da1afb15bfca748f17023abdddc731ca9abbbd05457` | nothing |
| `internal/gnovm/stdlibs/VERSION` | `internal/gnovm/stdlibs/VERSION` | third-party | 37 | 0.0% | 0 | `1252ea54d7bc6cd3d126e46df738b4c083e0f9f9bd647032dd9f34819241ea4a` | nothing |
| `internal/gnovm/stdlibs/bufio/bufio.gno` | `internal/gnovm/stdlibs/bufio/bufio.gno` | third-party | 19,896 | 0.0% | 0 | `aafd9975f577c6b62af82e2563bbe6b0c3580c011e2b38207e1a5d0463ec138f` | nothing |
| `internal/gnovm/stdlibs/bufio/scan.gno` | `internal/gnovm/stdlibs/bufio/scan.gno` | third-party | 14,004 | 0.0% | 0 | `3861e7b16e1aa2c751c4b4335893d1eb415e02183ccbc3f0b6fb48ee5f3dfca2` | nothing |
| `internal/gnovm/stdlibs/bytes/buffer.gno` | `internal/gnovm/stdlibs/bytes/buffer.gno` | third-party | 14,768 | 0.0% | 0 | `5c98e9db2d0df19bbb6f0da74c2d648012b79aa55bde8839123510ba49bfd010` | nothing |
| `internal/gnovm/stdlibs/bytes/bytes.gno` | `internal/gnovm/stdlibs/bytes/bytes.gno` | third-party | 29,792 | 0.0% | 0 | `da36c4ee60abc6feb61d01eafe5ee91dfa4775e1af6968f290329f9fb0e1e277` | nothing |
| `internal/gnovm/stdlibs/bytes/reader.gno` | `internal/gnovm/stdlibs/bytes/reader.gno` | third-party | 3,939 | 0.0% | 0 | `6e43236ffe8a1c6535f93629b743c3c5586301c8d742c1cb885d57fffef6a278` | nothing |
| `internal/gnovm/stdlibs/chain/address.gno` | `internal/gnovm/stdlibs/chain/address.gno` | third-party | 3,890 | 0.0% | 0 | `209af76730f89ac3b59b46609ba3b0817a5e221f935eba172390d01f67379c09` | nothing |
| `internal/gnovm/stdlibs/chain/banker/banker.gno` | `internal/gnovm/stdlibs/chain/banker/banker.gno` | third-party | 12,825 | 0.0% | 0 | `284866c09330453a6ce0695a2bf2e8296771c2321c2e7c97f4c73248a593647a` | nothing |
| `internal/gnovm/stdlibs/chain/coins.gno` | `internal/gnovm/stdlibs/chain/coins.gno` | third-party | 6,851 | 0.0% | 0 | `5dfb6ded5dee5deb6af70d9c193e82f8df64358769ad6bb1e830fabba774f0cc` | nothing |
| `internal/gnovm/stdlibs/chain/emit_event.gno` | `internal/gnovm/stdlibs/chain/emit_event.gno` | third-party | 1,139 | 0.0% | 0 | `955c7d24809736f557433a0efd76309c311d0b8af88da0a5e95dd7b103f3129a` | nothing |
| `internal/gnovm/stdlibs/chain/markdown/markdown.gno` | `internal/gnovm/stdlibs/chain/markdown/markdown.gno` | third-party | 6,973 | 0.0% | 0 | `ad5a8dcd31307c764c3b592ea6ef1ebb3075f24573d322833ef264749e6d12e5` | nothing |
| `internal/gnovm/stdlibs/chain/params/params.gno` | `internal/gnovm/stdlibs/chain/params/params.gno` | third-party | 2,191 | 0.0% | 0 | `09d1fedfc1eae09f371f1897e28d7a7276bcfd6038fa7c783b94ef52057c4cc3` | nothing |
| `internal/gnovm/stdlibs/chain/runtime/frame.gno` | `internal/gnovm/stdlibs/chain/runtime/frame.gno` | third-party | 3,118 | 0.0% | 0 | `2af15d2d286eecb4e893f485b1d6ba13fd9b23399109681df71dd25e151ace20` | nothing |
| `internal/gnovm/stdlibs/chain/runtime/native.gno` | `internal/gnovm/stdlibs/chain/runtime/native.gno` | third-party | 1,977 | 0.0% | 0 | `534187191cc9636625cc83e7b510ef3e3ccdab3644de4f93744a00bf48841507` | nothing |
| `internal/gnovm/stdlibs/chain/runtime/unsafe/unsafe.gno` | `internal/gnovm/stdlibs/chain/runtime/unsafe/unsafe.gno` | third-party | 3,059 | 0.0% | 0 | `1bb389cb46038537fb569af3d74e896ed30da39a3f9c2926c3178c73be243b51` | nothing |
| `internal/gnovm/stdlibs/crypto/bech32/bech32.gno` | `internal/gnovm/stdlibs/crypto/bech32/bech32.gno` | third-party | 14,788 | 0.0% | 0 | `f6d8e0ad91981a57ecd9a47f86e88bb603bcdf3ccc9ee5f29bcee271ab783d63` | nothing |
| `internal/gnovm/stdlibs/crypto/bech32/error.gno` | `internal/gnovm/stdlibs/crypto/bech32/error.gno` | third-party | 2,552 | 0.0% | 0 | `6d1c451fbb66a52352c60cb6f761c77256c0ea066dd87e83a833d9f5cd5051ce` | nothing |
| `internal/gnovm/stdlibs/crypto/bech32/version.gno` | `internal/gnovm/stdlibs/crypto/bech32/version.gno` | third-party | 1,152 | 0.0% | 0 | `4ae0d850650bb57a8c5476cf17b18c844c0178ee5ad3142e4da8808dd9d70df0` | nothing |
| `internal/gnovm/stdlibs/crypto/bn254/bn254.gno` | `internal/gnovm/stdlibs/crypto/bn254/bn254.gno` | third-party | 3,359 | 0.0% | 0 | `dd98311dab3eec14f3c156e472743529708c786d1cd2609abe8bb7f95dcc0a1a` | nothing |
| `internal/gnovm/stdlibs/crypto/chacha20/chacha/chacha.gno` | `internal/gnovm/stdlibs/crypto/chacha20/chacha/chacha.gno` | third-party | 5,708 | 0.0% | 0 | `0d0f3ed3e041032fcf810d2b0e54e7a04af5ca1048df406e2633cb01b5ff38d4` | nothing |
| `internal/gnovm/stdlibs/crypto/chacha20/chacha/chacha_generic.gno` | `internal/gnovm/stdlibs/crypto/chacha20/chacha/chacha_generic.gno` | third-party | 7,637 | 0.0% | 0 | `511b1ab283954cc94750f1fad481d41ec215428faeef6c9064f154abb78684c9` | nothing |
| `internal/gnovm/stdlibs/crypto/chacha20/chacha/chacha_ref.gno` | `internal/gnovm/stdlibs/crypto/chacha20/chacha/chacha_ref.gno` | third-party | 857 | 0.0% | 0 | `7dc2a004eea8b8cc05df2daed24b2ec64684a65589c33eababa33fc120f8d020` | nothing |
| `internal/gnovm/stdlibs/crypto/chacha20/chacha20.gno` | `internal/gnovm/stdlibs/crypto/chacha20/chacha20.gno` | third-party | 2,028 | 0.0% | 0 | `0e91b38f166dc744693cf5dbc87ee5fd05eb17d019d03174ac34669f8562862b` | nothing |
| `internal/gnovm/stdlibs/crypto/chacha20/rand/rand.gno` | `internal/gnovm/stdlibs/crypto/chacha20/rand/rand.gno` | third-party | 9,056 | 0.0% | 0 | `7d1828974b6d182928bb506a5c448247c0afae6bc310157c93e0a614b7e72b25` | nothing |
| `internal/gnovm/stdlibs/crypto/cipher/cipher.gno` | `internal/gnovm/stdlibs/crypto/cipher/cipher.gno` | third-party | 3,189 | 0.0% | 0 | `a38b464922afd477f9010d85bf62e205a1b0b383a1d3aa7737133ec33dba9932` | nothing |
| `internal/gnovm/stdlibs/crypto/cometbls/cometbls.gno` | `internal/gnovm/stdlibs/crypto/cometbls/cometbls.gno` | third-party | 2,592 | 0.0% | 0 | `365db7c9a5ae000793c04b4dcf695994a2d6467287abca3b554ec4c0ae2e265f` | nothing |
| `internal/gnovm/stdlibs/crypto/cometblszk/cometblszk.gno` | `internal/gnovm/stdlibs/crypto/cometblszk/cometblszk.gno` | third-party | 5,664 | 0.0% | 0 | `e970e53c8b1010e65cb5345605c0ba531c5c368533f787b730d22369d5ee017f` | nothing |
| `internal/gnovm/stdlibs/crypto/cometblszk/constants.gno` | `internal/gnovm/stdlibs/crypto/cometblszk/constants.gno` | third-party | 3,567 | 0.0% | 0 | `7427390e4d4c9a8cb5876b3204a173dbd730f670d4e25e941941adaf76ee4580` | nothing |
| `internal/gnovm/stdlibs/crypto/cometblszk/hashtofield.gno` | `internal/gnovm/stdlibs/crypto/cometblszk/hashtofield.gno` | third-party | 3,503 | 0.0% | 0 | `b6ba5034b97be02d98a569aab88db231de7d207587617c22166937895ed6fa98` | nothing |
| `internal/gnovm/stdlibs/crypto/cometblszk/verifier.gno` | `internal/gnovm/stdlibs/crypto/cometblszk/verifier.gno` | third-party | 4,400 | 0.0% | 0 | `32121e5caaa7e0edfe8fb1e87dbf8ea7b30ccd0bb17799195021a903d160ae9f` | nothing |
| `internal/gnovm/stdlibs/crypto/ed25519/ed25519.gno` | `internal/gnovm/stdlibs/crypto/ed25519/ed25519.gno` | third-party | 299 | 0.0% | 0 | `c82f6a6f41db2abb6ddb3300714816ed332898011ca0d04626969141bdc186b9` | nothing |
| `internal/gnovm/stdlibs/crypto/keccak256/keccak256.gno` | `internal/gnovm/stdlibs/crypto/keccak256/keccak256.gno` | third-party | 403 | 0.0% | 0 | `f0716694cf39bbfb8fcec265cd8e03b952d81df3772f3ffdca3904ddc3a2595e` | nothing |
| `internal/gnovm/stdlibs/crypto/merkle/merkle.gno` | `internal/gnovm/stdlibs/crypto/merkle/merkle.gno` | third-party | 1,003 | 0.0% | 0 | `b51c64a997153a33c357714f9086632e32a1e48a4eb1518e6494265626f707c1` | nothing |
| `internal/gnovm/stdlibs/crypto/modexp/modexp.gno` | `internal/gnovm/stdlibs/crypto/modexp/modexp.gno` | third-party | 2,164 | 0.0% | 0 | `4a308d4ebd4f7d958e817875e10fef61ebe5885ee97ae2590f68ccaa679e949d` | nothing |
| `internal/gnovm/stdlibs/crypto/sha256/sha256.gno` | `internal/gnovm/stdlibs/crypto/sha256/sha256.gno` | third-party | 189 | 0.0% | 0 | `36dcb3d6c365140d9d604bc228e36fd9e519196e90fcb1c9478ac1d788ed5a9c` | nothing |
| `internal/gnovm/stdlibs/crypto/subtle/xor.gno` | `internal/gnovm/stdlibs/crypto/subtle/xor.gno` | third-party | 1,501 | 0.0% | 0 | `650b925eab6de29af73be23f5e991f43f80746bf36dc794dca11cab2bc3fcad7` | nothing |
| `internal/gnovm/stdlibs/encoding/base32/base32.gno` | `internal/gnovm/stdlibs/encoding/base32/base32.gno` | third-party | 16,465 | 0.0% | 0 | `81e350c255d8e5fb2b10962bc55eb809fc09c67914b1316bfd1ee4bef46792cc` | nothing |
| `internal/gnovm/stdlibs/encoding/base64/base64.gno` | `internal/gnovm/stdlibs/encoding/base64/base64.gno` | third-party | 15,866 | 0.0% | 0 | `42d3d77983882ca6c27757b6d5d9897213386c78c9fb23dfd51028f65a6b4770` | nothing |
| `internal/gnovm/stdlibs/encoding/binary/binary.gno` | `internal/gnovm/stdlibs/encoding/binary/binary.gno` | third-party | 5,575 | 0.0% | 0 | `91c18bfdf20d33948cd2b9303724f33f8b66750bc214c697b228a9a4b35f5890` | nothing |
| `internal/gnovm/stdlibs/encoding/binary/varint.gno` | `internal/gnovm/stdlibs/encoding/binary/varint.gno` | third-party | 4,934 | 0.0% | 0 | `c2f162827797f552779f90fdcfd5632989d832eff54f9ade077daaa262a4d5bb` | nothing |
| `internal/gnovm/stdlibs/encoding/csv/reader.gno` | `internal/gnovm/stdlibs/encoding/csv/reader.gno` | third-party | 14,522 | 0.0% | 0 | `3b94ef8c1196f2c43757b59a88a00eef8c7127dc64395d762fafd89b332c76c2` | nothing |
| `internal/gnovm/stdlibs/encoding/csv/writer.gno` | `internal/gnovm/stdlibs/encoding/csv/writer.gno` | third-party | 4,962 | 0.0% | 0 | `c9392233011d33048788258a077ea5cec9932fcef0610554b1824a596c13c715` | nothing |
| `internal/gnovm/stdlibs/encoding/encoding.gno` | `internal/gnovm/stdlibs/encoding/encoding.gno` | third-party | 2,285 | 0.0% | 0 | `059391ed814719210c5dc1a7672c657814ec6c16c250150682003bd922325a40` | nothing |
| `internal/gnovm/stdlibs/encoding/hex/hex.gno` | `internal/gnovm/stdlibs/encoding/hex/hex.gno` | third-party | 8,415 | 0.0% | 0 | `a759c69b593b8fb136b8641a44a578f233a3fed26f7c66d3db7a4fde310abe01` | nothing |
| `internal/gnovm/stdlibs/errors/errors.gno` | `internal/gnovm/stdlibs/errors/errors.gno` | third-party | 1,730 | 0.0% | 0 | `dd9769f2955e37e1039fb49f8981a61470f3228db3140073223f1b9ce5ffa7e8` | nothing |
| `internal/gnovm/stdlibs/errors/join.gno` | `internal/gnovm/stdlibs/errors/join.gno` | third-party | 1,112 | 0.0% | 0 | `1696f984e2d5825086ec3a718950399010322bb67fe6e56d8da7f9f50520b603` | nothing |
| `internal/gnovm/stdlibs/errors/wrap.gno` | `internal/gnovm/stdlibs/errors/wrap.gno` | third-party | 2,533 | 0.0% | 0 | `6ea8c5ec514a5ed0076858bcfcbd518a93124db1acf2f71b5e8589e2bfae9da4` | nothing |
| `internal/gnovm/stdlibs/hash/adler32/adler32.gno` | `internal/gnovm/stdlibs/hash/adler32/adler32.gno` | third-party | 3,259 | 0.0% | 0 | `303eb0589b544da9a5b8a1d2b9e1c30645ccec67182c354cbcc15c3290c857e2` | nothing |
| `internal/gnovm/stdlibs/hash/hash.gno` | `internal/gnovm/stdlibs/hash/hash.gno` | third-party | 2,138 | 0.0% | 0 | `992bf264e591b27b543e60064fafb5646822e424d132693dab0b80ce55f5c013` | nothing |
| `internal/gnovm/stdlibs/html/entity.gno` | `internal/gnovm/stdlibs/html/entity.gno` | third-party | 114,577 | 0.0% | 0 | `72fdaa14bb63bfc44b2f8976bd0a84acf13b787b67098e6639ac304a52907a36` | nothing |
| `internal/gnovm/stdlibs/html/escape.gno` | `internal/gnovm/stdlibs/html/escape.gno` | third-party | 5,123 | 0.0% | 0 | `b0a2e9c1930561fadcb5c3e44543845f99341f9dac5f6677e3ccaaab2dba10f1` | nothing |
| `internal/gnovm/stdlibs/internal/bytealg/bytealg.gno` | `internal/gnovm/stdlibs/internal/bytealg/bytealg.gno` | third-party | 3,827 | 0.0% | 0 | `0a6a5de1985210a2fd012f69777466f322c18ed60180e53848a03c911808b94d` | nothing |
| `internal/gnovm/stdlibs/internal/bytealg/compare_generic.gno` | `internal/gnovm/stdlibs/internal/bytealg/compare_generic.gno` | third-party | 851 | 0.0% | 0 | `4b66f4868a9ef72177adfec0a52e152a40ce771e3499816023086a93dc1669f7` | nothing |
| `internal/gnovm/stdlibs/internal/bytealg/count_generic.gno` | `internal/gnovm/stdlibs/internal/bytealg/count_generic.gno` | third-party | 407 | 0.0% | 0 | `13c0a0b6211eb47288a1b0e1ddf5b140b430ffefa17e1f601c062dc48a53771f` | nothing |
| `internal/gnovm/stdlibs/internal/bytealg/equal_generic.gno` | `internal/gnovm/stdlibs/internal/bytealg/equal_generic.gno` | third-party | 628 | 0.0% | 0 | `d6c5c106505c972fab2400024c14e04aa7eee1f972826e3b56787ee09f169247` | nothing |
| `internal/gnovm/stdlibs/internal/bytealg/index_generic.gno` | `internal/gnovm/stdlibs/internal/bytealg/index_generic.gno` | third-party | 840 | 0.0% | 0 | `1e47df67f805085dc274d6cf1733d4b6c5654eadbe160b548cf220708bf4e3ae` | nothing |
| `internal/gnovm/stdlibs/internal/bytealg/indexbyte_generic.gno` | `internal/gnovm/stdlibs/internal/bytealg/indexbyte_generic.gno` | third-party | 411 | 0.0% | 0 | `218796bc9e9a835c99188c9d281c77e387f16374b6d21c23c75cf7703530d97f` | nothing |
| `internal/gnovm/stdlibs/io/io.gno` | `internal/gnovm/stdlibs/io/io.gno` | third-party | 21,857 | 0.0% | 0 | `edb2c321dc2a50ea942a4aa09d6c76b809f7fd60d70b146d4c3711de68640109` | nothing |
| `internal/gnovm/stdlibs/io/multi.gno` | `internal/gnovm/stdlibs/io/multi.gno` | third-party | 2,746 | 0.0% | 0 | `468b04495d4a3be27e61646c90ef5655ea53f3348ee0f66f0248edcb4d18b834` | nothing |
| `internal/gnovm/stdlibs/math/abs.gno` | `internal/gnovm/stdlibs/math/abs.gno` | third-party | 366 | 0.0% | 0 | `9608d0f0e20d941025c2bf268e8541dbecaf7e9b1524e030c390166384a7da96` | nothing |
| `internal/gnovm/stdlibs/math/acosh.gno` | `internal/gnovm/stdlibs/math/acosh.gno` | third-party | 1,704 | 0.0% | 0 | `99848f18314cf7409d3cc301cc551d4fd5ec439e7b21d90d5f6f425890d848b6` | nothing |
| `internal/gnovm/stdlibs/math/asin.gno` | `internal/gnovm/stdlibs/math/asin.gno` | third-party | 1,029 | 0.0% | 0 | `1dc2d8003a61687f319475b88ee0571462b0d6932be10cf5700aaeb6c302dcfe` | nothing |
| `internal/gnovm/stdlibs/math/asinh.gno` | `internal/gnovm/stdlibs/math/asinh.gno` | third-party | 1,922 | 0.0% | 0 | `09119cd8282a7ae3eba756857965aadbc135067455dd65e61a899a09d87bd128` | nothing |
| `internal/gnovm/stdlibs/math/atan.gno` | `internal/gnovm/stdlibs/math/atan.gno` | third-party | 3,056 | 0.0% | 0 | `f0d17c9892842e5459f36cd19d7b064ef77380d48b370de6e009855d4733a19e` | nothing |
| `internal/gnovm/stdlibs/math/atan2.gno` | `internal/gnovm/stdlibs/math/atan2.gno` | third-party | 1,512 | 0.0% | 0 | `7c5eeba71a479149a05ce7c23cff7c799a7d281d33bbecb5902b79039ddc663b` | nothing |
| `internal/gnovm/stdlibs/math/atanh.gno` | `internal/gnovm/stdlibs/math/atanh.gno` | third-party | 1,997 | 0.0% | 0 | `2c507853ca092de4ddba94cecbc22f8d8c3ff91f6a7709ae3f2763787424a775` | nothing |
| `internal/gnovm/stdlibs/math/bits.gno` | `internal/gnovm/stdlibs/math/bits.gno` | third-party | 1,915 | 0.0% | 0 | `83cd55c5b778fff63f43686f894551279c9a9f25cd8d8a7deb7030e83f7689dc` | nothing |
| `internal/gnovm/stdlibs/math/bits/bits.gno` | `internal/gnovm/stdlibs/math/bits/bits.gno` | third-party | 18,307 | 0.0% | 0 | `a1d9f7908b3abc05a1fef37f99525e99a69cda62ed7067442e6052e7fdec63e1` | nothing |
| `internal/gnovm/stdlibs/math/bits/bits_errors.gno` | `internal/gnovm/stdlibs/math/bits/bits_errors.gno` | third-party | 325 | 0.0% | 0 | `5c974c4e970d1019832a06c804e93d0fdd6df2b4a4451e53a2038bfb49354b76` | nothing |
| `internal/gnovm/stdlibs/math/bits/bits_tables.gno` | `internal/gnovm/stdlibs/math/bits/bits_tables.gno` | third-party | 4,791 | 0.0% | 0 | `c4cb4a05dab6d395d9d51720893a42f0a9d1cdf8a0c2383d87268e49acb73f7f` | nothing |
| `internal/gnovm/stdlibs/math/cbrt.gno` | `internal/gnovm/stdlibs/math/cbrt.gno` | third-party | 2,323 | 0.0% | 0 | `bde23d983dba3200e62219681ea7fbe72db1b593650dc545e5707f0e0451cffb` | nothing |
| `internal/gnovm/stdlibs/math/const.gno` | `internal/gnovm/stdlibs/math/const.gno` | third-party | 2,732 | 0.0% | 0 | `d0117ada25890ec6176227512ee5d28b10f53efc3591f221a76aa40a1d4dd539` | nothing |
| `internal/gnovm/stdlibs/math/copysign.gno` | `internal/gnovm/stdlibs/math/copysign.gno` | third-party | 396 | 0.0% | 0 | `9d7e6179f8bf2df156b1bd054de7ef12afe26205c1b786e3ec059b1a80284bda` | nothing |
| `internal/gnovm/stdlibs/math/dim.gno` | `internal/gnovm/stdlibs/math/dim.gno` | third-party | 1,637 | 0.0% | 0 | `d9e16ce4924a3bd338bd4826d2b00008a82ff2ffd6a96bfe4ab15f1291e3077a` | nothing |
| `internal/gnovm/stdlibs/math/erf.gno` | `internal/gnovm/stdlibs/math/erf.gno` | third-party | 11,701 | 0.0% | 0 | `878e7bbb29d10d49bdd935a01164dd34425d9ca5ca7ba923bf2d491f3bc40c5c` | nothing |
| `internal/gnovm/stdlibs/math/erfinv.gno` | `internal/gnovm/stdlibs/math/erfinv.gno` | third-party | 3,448 | 0.0% | 0 | `1c6114733176cc615eb5ee0a66a700f76f4dee2a5b69311948b8af2afa417d6c` | nothing |
| `internal/gnovm/stdlibs/math/exp.gno` | `internal/gnovm/stdlibs/math/exp.gno` | third-party | 5,419 | 0.0% | 0 | `5e52dd5be1c74de17b4b6eeda4ce60ae4bfb450b5650447890bbc761385a1bf4` | nothing |
| `internal/gnovm/stdlibs/math/expm1.gno` | `internal/gnovm/stdlibs/math/expm1.gno` | third-party | 8,052 | 0.0% | 0 | `f3951d21cb4d1209b54087580bb6b24c4b0a5fcf6c277984e89a79f33d3e219a` | nothing |
| `internal/gnovm/stdlibs/math/floor.gno` | `internal/gnovm/stdlibs/math/floor.gno` | third-party | 3,237 | 0.0% | 0 | `fe856deac0edb156e30fc1f5e75dfe4104a535cc760b6e10a1f0204edb48fef0` | nothing |
| `internal/gnovm/stdlibs/math/fma.gno` | `internal/gnovm/stdlibs/math/fma.gno` | third-party | 4,719 | 0.0% | 0 | `2b670f3d1549bc868322d3b6473c91e739cc94f70b52fc9a890a444db6ab1766` | nothing |
| `internal/gnovm/stdlibs/math/frexp.gno` | `internal/gnovm/stdlibs/math/frexp.gno` | third-party | 884 | 0.0% | 0 | `f9876ae035980ebc06834478fae1a1c374c8ea14c2f7e70edbd5e24123d90018` | nothing |
| `internal/gnovm/stdlibs/math/gamma.gno` | `internal/gnovm/stdlibs/math/gamma.gno` | third-party | 5,660 | 0.0% | 0 | `17dbe0483b6d558ca976768c0ca8309d465ad2c1b9c1f6bb072e72e552d272ef` | nothing |
| `internal/gnovm/stdlibs/math/hypot.gno` | `internal/gnovm/stdlibs/math/hypot.gno` | third-party | 800 | 0.0% | 0 | `af1d3d17e3080dc34f3a1aca2f706a61dc5dad5ade3e2402f6d2f6cff647e888` | nothing |
| `internal/gnovm/stdlibs/math/j0.gno` | `internal/gnovm/stdlibs/math/j0.gno` | third-party | 13,939 | 0.0% | 0 | `34d612118f92d9d95d315579838a9e7a5dfc283ae4eb8f23723300ab90db06d1` | nothing |
| `internal/gnovm/stdlibs/math/j1.gno` | `internal/gnovm/stdlibs/math/j1.gno` | third-party | 13,631 | 0.0% | 0 | `c5f276826a484feadb8fe7416f38c9ebffbaecbdde0c189c084ace137753cf05` | nothing |
| `internal/gnovm/stdlibs/math/jn.gno` | `internal/gnovm/stdlibs/math/jn.gno` | third-party | 7,351 | 0.0% | 0 | `b588c5389aa3ac04eeb72b29a610bef4af02b8ee7faf3d50717d2da2219bc144` | nothing |
| `internal/gnovm/stdlibs/math/ldexp.gno` | `internal/gnovm/stdlibs/math/ldexp.gno` | third-party | 1,022 | 0.0% | 0 | `1bf941c9bd8ef79ac56abf7dcce1fe96ee5ba197b2f4e6b0cc0c0ef8da3f0313` | nothing |
| `internal/gnovm/stdlibs/math/lgamma.gno` | `internal/gnovm/stdlibs/math/lgamma.gno` | third-party | 11,294 | 0.0% | 0 | `e07996b7b64d6e9b8edfed45cf2ec1b47b2bd9d1872d46dabebe8d27ea9058c5` | nothing |
| `internal/gnovm/stdlibs/math/log.gno` | `internal/gnovm/stdlibs/math/log.gno` | third-party | 3,912 | 0.0% | 0 | `387ced0bc4d6a5b546b5f9b908536fa4a50fc13e1373d36bbc713f98b4d31c10` | nothing |
| `internal/gnovm/stdlibs/math/log10.gno` | `internal/gnovm/stdlibs/math/log10.gno` | third-party | 781 | 0.0% | 0 | `e597c591df1671aebaee4d832a1718a537006248df1ce38b568a20b2a363d274` | nothing |
| `internal/gnovm/stdlibs/math/log1p.gno` | `internal/gnovm/stdlibs/math/log1p.gno` | third-party | 6,450 | 0.0% | 0 | `567a994f0ebb98f30828da09055b9a29f238e3aaa205f2827d4cb1980eeb3c5d` | nothing |
| `internal/gnovm/stdlibs/math/logb.gno` | `internal/gnovm/stdlibs/math/logb.gno` | third-party | 1,021 | 0.0% | 0 | `9cd63343d6d38130520af9e1a107b5bce1e670357a6292e157fcab90c7b82365` | nothing |
| `internal/gnovm/stdlibs/math/mod.gno` | `internal/gnovm/stdlibs/math/mod.gno` | third-party | 859 | 0.0% | 0 | `3e0b1955e0c8f111f9d472703d5feaef78bcfeb274e7a9c2cf20602eaffd3714` | nothing |
| `internal/gnovm/stdlibs/math/modf.gno` | `internal/gnovm/stdlibs/math/modf.gno` | third-party | 870 | 0.0% | 0 | `d275b2cd2469c1b63338c1582826207113f114b4735348bc675aea37db6527fc` | nothing |
| `internal/gnovm/stdlibs/math/native.gno` | `internal/gnovm/stdlibs/math/native.gno` | third-party | 989 | 0.0% | 0 | `393837ce52b5798d91be50271057b618767a317d3d8816eaacb325fc2e2d4f44` | nothing |
| `internal/gnovm/stdlibs/math/nextafter.gno` | `internal/gnovm/stdlibs/math/nextafter.gno` | third-party | 1,239 | 0.0% | 0 | `f52cd1442fadfca48ff5f3ea8620293655180486e3ad1f88288568c797f29e15` | nothing |
| `internal/gnovm/stdlibs/math/overflow/doc.gno` | `internal/gnovm/stdlibs/math/overflow/doc.gno` | third-party | 716 | 0.0% | 0 | `e036997ba94e4ec200c3aa599f6109d9ce1d8823612c5d78d0297422270903e4` | nothing |
| `internal/gnovm/stdlibs/math/overflow/overflow_generated.gno` | `internal/gnovm/stdlibs/math/overflow/overflow_generated.gno` | third-party | 14,871 | 0.0% | 0 | `d92510f1ed703e6a19528220676001580e312daea18fff4ab4e145ff31317b37` | nothing |
| `internal/gnovm/stdlibs/math/pow.gno` | `internal/gnovm/stdlibs/math/pow.gno` | third-party | 3,690 | 0.0% | 0 | `b9b99cb79344b88b3e2a38b11812d09541b2f51df27c3ae832a1b86bd871d90a` | nothing |
| `internal/gnovm/stdlibs/math/pow10.gno` | `internal/gnovm/stdlibs/math/pow10.gno` | third-party | 1,269 | 0.0% | 0 | `75c48d71fbe5930b75f736688145efa2f6373938ce988b7bd659d5ee273378d3` | nothing |
| `internal/gnovm/stdlibs/math/rand/exp.gno` | `internal/gnovm/stdlibs/math/rand/exp.gno` | third-party | 11,212 | 0.0% | 0 | `906b6dd4e8605f4a6974ec357a30b9bd0e00cb249f484f5a1ea44892c4b7b5eb` | nothing |
| `internal/gnovm/stdlibs/math/rand/normal.gno` | `internal/gnovm/stdlibs/math/rand/normal.gno` | third-party | 6,472 | 0.0% | 0 | `9a1fc432f84d343fed2d43eafa118fb37a1428f78b04775c549a4850885f8ed2` | nothing |
| `internal/gnovm/stdlibs/math/rand/pcg.gno` | `internal/gnovm/stdlibs/math/rand/pcg.gno` | third-party | 3,539 | 0.0% | 0 | `29628d63f8aeed5ea60f711b23e78269ee3c330ed67754eccdd54ea9f2551f6e` | nothing |
| `internal/gnovm/stdlibs/math/rand/rand.gno` | `internal/gnovm/stdlibs/math/rand/rand.gno` | third-party | 12,296 | 0.0% | 0 | `d24bc8a7c8c5165e93bfa21eda9087d46c8dcb830b96c2de5db2c6b2c3a9356d` | nothing |
| `internal/gnovm/stdlibs/math/rand/zipf.gno` | `internal/gnovm/stdlibs/math/rand/zipf.gno` | third-party | 1,845 | 0.0% | 0 | `ee4616e17c8b1d7ff33229b075f2e73c369307be02258f173393508cb2db0357` | nothing |
| `internal/gnovm/stdlibs/math/remainder.gno` | `internal/gnovm/stdlibs/math/remainder.gno` | third-party | 2,036 | 0.0% | 0 | `aaf124ea06e724febf24963542653a5995dc6e3883d69cb7dd122df6d243dda5` | nothing |
| `internal/gnovm/stdlibs/math/signbit.gno` | `internal/gnovm/stdlibs/math/signbit.gno` | third-party | 302 | 0.0% | 0 | `f2a9103555eadab80b9d0ca293de87577fab5f3c4cdf0d817648afff480fa24d` | nothing |
| `internal/gnovm/stdlibs/math/sin.gno` | `internal/gnovm/stdlibs/math/sin.gno` | third-party | 6,423 | 0.0% | 0 | `3d868a11f488c6f6998e4f4cead509f59512db9432f7c59bf808afcbe1784b7c` | nothing |
| `internal/gnovm/stdlibs/math/sincos.gno` | `internal/gnovm/stdlibs/math/sincos.gno` | third-party | 1,800 | 0.0% | 0 | `d1a9614979455110d808aa194fa6e2451cbfe837b11584311fbb356a664a52e5` | nothing |
| `internal/gnovm/stdlibs/math/sinh.gno` | `internal/gnovm/stdlibs/math/sinh.gno` | third-party | 1,647 | 0.0% | 0 | `b6c3958eddbecb2b1d8221e511fbc5685f4c393b3de9b982b4ea1d94877afd2e` | nothing |
| `internal/gnovm/stdlibs/math/sqrt.gno` | `internal/gnovm/stdlibs/math/sqrt.gno` | third-party | 4,865 | 0.0% | 0 | `c7e036d1511bea94992014f9b8c9259e5737c385cdd5135db9092369250d4a94` | nothing |
| `internal/gnovm/stdlibs/math/tan.gno` | `internal/gnovm/stdlibs/math/tan.gno` | third-party | 3,728 | 0.0% | 0 | `d3eefe31bd8acb1375ca7310ec7b7433c5592f52fca058bafca3c11338b76a82` | nothing |
| `internal/gnovm/stdlibs/math/tanh.gno` | `internal/gnovm/stdlibs/math/tanh.gno` | third-party | 2,679 | 0.0% | 0 | `80d2eb22885fc5fc54b86c32fbe41c328f3094588360a02b43ac604cfb81cc32` | nothing |
| `internal/gnovm/stdlibs/math/trig_reduce.gno` | `internal/gnovm/stdlibs/math/trig_reduce.gno` | third-party | 3,419 | 0.0% | 0 | `fa54b4e9e792707630cd402f19e742ba23c574dc6255867a395e737494a61595` | nothing |
| `internal/gnovm/stdlibs/net/url/url.gno` | `internal/gnovm/stdlibs/net/url/url.gno` | third-party | 35,528 | 0.0% | 0 | `cadc180b8402b9435de40407cd8b772484fc004f3eab06e0317c15ceca3311ec` | nothing |
| `internal/gnovm/stdlibs/path/match.gno` | `internal/gnovm/stdlibs/path/match.gno` | third-party | 5,464 | 0.0% | 0 | `ad7d6133c7d6c75b8a6eed5952a4e2db90afd030de4aa95350ba2278feaedf25` | nothing |
| `internal/gnovm/stdlibs/path/path.gno` | `internal/gnovm/stdlibs/path/path.gno` | third-party | 6,099 | 0.0% | 0 | `58492c35df3bd9984d6010780e7873c1cb11f55716d6766702a147f15115ec7a` | nothing |
| `internal/gnovm/stdlibs/regexp/backtrack.gno` | `internal/gnovm/stdlibs/regexp/backtrack.gno` | third-party | 9,033 | 0.0% | 0 | `525595e75d953f8be77aa0ff6c438c9b43e8b6220980709e5e11852cfac6d4fa` | nothing |
| `internal/gnovm/stdlibs/regexp/exec.gno` | `internal/gnovm/stdlibs/regexp/exec.gno` | third-party | 12,614 | 0.0% | 0 | `ceaed324d06dc7422a152abd1d1fc8d54429f66efd745c309f4d7fa8e436e763` | nothing |
| `internal/gnovm/stdlibs/regexp/onepass.gno` | `internal/gnovm/stdlibs/regexp/onepass.gno` | third-party | 14,319 | 0.0% | 0 | `816a6bacd5e59c771642729543b6deaf85f0a9f067804ab235386d897847cd28` | nothing |
| `internal/gnovm/stdlibs/regexp/regexp.gno` | `internal/gnovm/stdlibs/regexp/regexp.gno` | third-party | 38,479 | 0.0% | 0 | `c8e9570da4cabff5ce57afab7968a77e7de520fa4a0af4271315655ff6a16ec9` | nothing |
| `internal/gnovm/stdlibs/regexp/syntax/compile.gno` | `internal/gnovm/stdlibs/regexp/syntax/compile.gno` | third-party | 6,998 | 0.0% | 0 | `d17fc96c442fda1fe956b7e5134e41656c6e4fceef431d235d1ce56794c84390` | nothing |
| `internal/gnovm/stdlibs/regexp/syntax/doc.gno` | `internal/gnovm/stdlibs/regexp/syntax/doc.gno` | third-party | 5,400 | 0.0% | 0 | `4c4149ea9d32362331b85d32a7fcaf846a32bf1fb639ff025b8e4e3ca9cbc4a1` | nothing |
| `internal/gnovm/stdlibs/regexp/syntax/op_string.gno` | `internal/gnovm/stdlibs/regexp/syntax/op_string.gno` | third-party | 658 | 0.0% | 0 | `a51ecaa7f7c16f2f4e241e9375f9d6c2f3e889d6f55eb52639517a4deaa8db75` | nothing |
| `internal/gnovm/stdlibs/regexp/syntax/parse.gno` | `internal/gnovm/stdlibs/regexp/syntax/parse.gno` | third-party | 47,283 | 0.0% | 0 | `260a73c1dd244e05b223ec1b8ba700e5ad5ec8dd6e81ac11a4cf4fa8473e0613` | nothing |
| `internal/gnovm/stdlibs/regexp/syntax/perl_groups.gno` | `internal/gnovm/stdlibs/regexp/syntax/perl_groups.gno` | third-party | 2,404 | 0.0% | 0 | `c7584c476c9d823c6a1c32ab7629b96241656b3b0e64d1a11c8a53fa6a68e6cd` | nothing |
| `internal/gnovm/stdlibs/regexp/syntax/prog.gno` | `internal/gnovm/stdlibs/regexp/syntax/prog.gno` | third-party | 7,640 | 0.0% | 0 | `878aa343df6c711120252b48e57c538768317a79b32473b2d4c22adf50954e52` | nothing |
| `internal/gnovm/stdlibs/regexp/syntax/regexp.gno` | `internal/gnovm/stdlibs/regexp/syntax/regexp.gno` | third-party | 7,854 | 0.0% | 0 | `001039ca7db05378c00e360cff3717d8d06965c26065d3832effc1c8ec014966` | nothing |
| `internal/gnovm/stdlibs/regexp/syntax/simplify.gno` | `internal/gnovm/stdlibs/regexp/syntax/simplify.gno` | third-party | 4,299 | 0.0% | 0 | `d22b48ecde668bc22cd8998492c752ec088da5d027d61b7f04f7a9298796eec0` | nothing |
| `internal/gnovm/stdlibs/sort/search.gno` | `internal/gnovm/stdlibs/sort/search.gno` | third-party | 4,227 | 0.0% | 0 | `b49769efeff7e2cf144b3c94a3c59f30b64efb4d5db41984b13a51beae420c80` | nothing |
| `internal/gnovm/stdlibs/sort/sort.gno` | `internal/gnovm/stdlibs/sort/sort.gno` | third-party | 17,330 | 0.0% | 0 | `13bd13fb30f0788362bee94ee4f788f458bc09fde542ca4bcbdc49daa048a05b` | nothing |
| `internal/gnovm/stdlibs/strconv/atob.gno` | `internal/gnovm/stdlibs/strconv/atob.gno` | third-party | 974 | 0.0% | 0 | `a811df1f66377d76716b133aaa0e0029f8b8432f39b9128e60aeb792c5e70fdb` | nothing |
| `internal/gnovm/stdlibs/strconv/atof.gno` | `internal/gnovm/stdlibs/strconv/atof.gno` | third-party | 16,281 | 0.0% | 0 | `c8d9a1720134e2f42816f74a4925f2f062ee17694698cc9a6add5471e920bde4` | nothing |
| `internal/gnovm/stdlibs/strconv/atoi.gno` | `internal/gnovm/stdlibs/strconv/atoi.gno` | third-party | 8,802 | 0.0% | 0 | `afea9d3addcfd2ff916acfdfe5f9ab5437d77a6a5ffc24433fdec3eb19f438ec` | nothing |
| `internal/gnovm/stdlibs/strconv/bytealg.gno` | `internal/gnovm/stdlibs/strconv/bytealg.gno` | third-party | 357 | 0.0% | 0 | `9c5fb8debb0e7673fe5b99d59c64464a33dcebd4d279eb20979c6ff59d1293c3` | nothing |
| `internal/gnovm/stdlibs/strconv/decimal.gno` | `internal/gnovm/stdlibs/strconv/decimal.gno` | third-party | 11,299 | 0.0% | 0 | `23bd45989d0b5a7b441c45873bf43f210fdf8381132fc74424d381216ac3a8b4` | nothing |
| `internal/gnovm/stdlibs/strconv/doc.gno` | `internal/gnovm/stdlibs/strconv/doc.gno` | third-party | 2,103 | 0.0% | 0 | `7449868a9a5ad773f29644295e928472884f55ed8dd11a3b434c55ce49e66c5e` | nothing |
| `internal/gnovm/stdlibs/strconv/eisel_lemire.gno` | `internal/gnovm/stdlibs/strconv/eisel_lemire.gno` | third-party | 42,385 | 0.0% | 0 | `702cdbff7add80c64c79178226355c6c4fd228c19e31c0dac4479fe4b476e662` | nothing |
| `internal/gnovm/stdlibs/strconv/ftoa.gno` | `internal/gnovm/stdlibs/strconv/ftoa.gno` | third-party | 14,341 | 0.0% | 0 | `a17a6c79dcdfab98e63756d6342ec6287ddcb193aaafbe44e62d74e31c358745` | nothing |
| `internal/gnovm/stdlibs/strconv/ftoaryu.gno` | `internal/gnovm/stdlibs/strconv/ftoaryu.gno` | third-party | 16,036 | 0.0% | 0 | `369cf8c283e9ca407d26b7faa7b2a91ee29bb75a535b7591f132dd8d1908932f` | nothing |
| `internal/gnovm/stdlibs/strconv/isprint.gno` | `internal/gnovm/stdlibs/strconv/isprint.gno` | third-party | 11,824 | 0.0% | 0 | `24114557c2b3cad7b80c8e71683831bfbe1c72eb99e8fd28a3a48eaf77705fb4` | nothing |
| `internal/gnovm/stdlibs/strconv/itoa.gno` | `internal/gnovm/stdlibs/strconv/itoa.gno` | third-party | 5,428 | 0.0% | 0 | `8d24d4cefb5a172455f30669ecc47840d962eb317e18e71ff533fdbc6c8b5056` | nothing |
| `internal/gnovm/stdlibs/strconv/quote.gno` | `internal/gnovm/stdlibs/strconv/quote.gno` | third-party | 17,188 | 0.0% | 0 | `78295f53fbcf77266ceae2f2c9321fecbf1c3183e512d43875d1171dc6a6589d` | nothing |
| `internal/gnovm/stdlibs/strings/builder.gno` | `internal/gnovm/stdlibs/strings/builder.gno` | third-party | 3,848 | 0.0% | 0 | `b90bf83fa155157db500ba8b0c46acc39fdde3dda5ee0c54f876972704702aa0` | nothing |
| `internal/gnovm/stdlibs/strings/compare.gno` | `internal/gnovm/stdlibs/strings/compare.gno` | third-party | 1,081 | 0.0% | 0 | `3933f7a957d05cfcaa0a30114540def715dd00eacf75465226205892a43097bc` | nothing |
| `internal/gnovm/stdlibs/strings/reader.gno` | `internal/gnovm/stdlibs/strings/reader.gno` | third-party | 3,965 | 0.0% | 0 | `bd6d135d3599b16e977bcd0283cf4c57afd1662c0270488ba9c699daea84f7e0` | nothing |
| `internal/gnovm/stdlibs/strings/replace.gno` | `internal/gnovm/stdlibs/strings/replace.gno` | third-party | 14,729 | 0.0% | 0 | `a2bf081843e2de1f609c2dcdbd3ced351e5d6cff65db25167033bf6596d6fb16` | nothing |
| `internal/gnovm/stdlibs/strings/search.gno` | `internal/gnovm/stdlibs/strings/search.gno` | third-party | 4,291 | 0.0% | 0 | `9d4f68ff30cb78fbccfd950486d859723004f8df917fe8ba95993f8e8fe544e1` | nothing |
| `internal/gnovm/stdlibs/strings/strings.gno` | `internal/gnovm/stdlibs/strings/strings.gno` | third-party | 28,550 | 0.0% | 0 | `4320510164322e4e9f6f19136fa958cf104ead19df40ce930a86890cd86949a4` | nothing |
| `internal/gnovm/stdlibs/sys/params/params.gno` | `internal/gnovm/stdlibs/sys/params/params.gno` | third-party | 3,205 | 0.0% | 0 | `36fcdd33600df59295947e8d6731ca961ba52c8ae8f56d6b4edc8d3cb2a3e77c` | nothing |
| `internal/gnovm/stdlibs/time/format.gno` | `internal/gnovm/stdlibs/time/format.gno` | third-party | 46,897 | 0.0% | 0 | `5aae8ac4836e7ffa0a56f14d4721c1f105f1705668fface5c1494cdfcb9c353c` | nothing |
| `internal/gnovm/stdlibs/time/time.gno` | `internal/gnovm/stdlibs/time/time.gno` | third-party | 40,387 | 0.0% | 0 | `db11a5f53a535199682e5dac08c41bc72968dbde42b5fdf93e0a19007f828d48` | nothing |
| `internal/gnovm/stdlibs/time/timezoneinfo.gno` | `internal/gnovm/stdlibs/time/timezoneinfo.gno` | third-party | 17,215 | 0.0% | 0 | `f53fa325530fa3a67496bf0b7ab931926a4fc6e4b648797237a2169e52cebf1d` | nothing |
| `internal/gnovm/stdlibs/time/zoneinfo_read.gno` | `internal/gnovm/stdlibs/time/zoneinfo_read.gno` | third-party | 8,076 | 0.0% | 0 | `71b15cd0482cd3b382e5b2c50bf8e452ea81412cade11a164a0f9d6f7bfbdebf` | nothing |
| `internal/gnovm/stdlibs/unicode/casetables.gno` | `internal/gnovm/stdlibs/unicode/casetables.gno` | third-party | 756 | 0.0% | 0 | `4e6e9aacb016e4151baedba0732fdae1e3e64dbe70f1c91b69d02f5e62d36d8b` | nothing |
| `internal/gnovm/stdlibs/unicode/digit.gno` | `internal/gnovm/stdlibs/unicode/digit.gno` | third-party | 352 | 0.0% | 0 | `dfb14dbbcb918a6c7ee9be15293327d022f9d64a4c1a90f9c8049b682e799761` | nothing |
| `internal/gnovm/stdlibs/unicode/graphic.gno` | `internal/gnovm/stdlibs/unicode/graphic.gno` | third-party | 4,463 | 0.0% | 0 | `b283c5381270c83a952e9b073c82ddb5e603c15a21d95a765009e09c27691c87` | nothing |
| `internal/gnovm/stdlibs/unicode/letter.gno` | `internal/gnovm/stdlibs/unicode/letter.gno` | third-party | 10,186 | 0.0% | 0 | `150c85cdff584007484b132c2a32c948e4809bdf1f3caba58984a1d26529a8c9` | nothing |
| `internal/gnovm/stdlibs/unicode/tables.gno` | `internal/gnovm/stdlibs/unicode/tables.gno` | third-party | 210,106 | 0.0% | 0 | `1e5650761f563298ab74e7c514fd53f081a9daa0bb2bf37664df380952796686` | nothing |
| `internal/gnovm/stdlibs/unicode/utf16/utf16.gno` | `internal/gnovm/stdlibs/unicode/utf16/utf16.gno` | third-party | 3,437 | 0.0% | 0 | `71b0410752b72c7c2d04cea06d68f8cce5b1c5387cc3e5cb3de8b294a16af4be` | nothing |
| `internal/gnovm/stdlibs/unicode/utf8/utf8.gno` | `internal/gnovm/stdlibs/unicode/utf8/utf8.gno` | third-party | 16,811 | 0.0% | 0 | `8070cdd409b30d1c15094f079ab0f0496f3d731d12d3b4b04ad26c422adf4113` | nothing |
| `internal/gnovm/stdlibs_test.go` | `internal/gnovm/stdlibs_test.go` | carried | 3,333 | 0.0% | 0 | `8818673700b85ff69f02681717534936966867d9294137a78e315bea27fc73b3` | nothing |
| `internal/gnovm/testdata/gnomem/v2/frames.golden` | `internal/gnovm/testdata/gnomem/v2/frames.golden` | carried | 30,783 | 0.0% | 0 | `e3cd54f9529b2fa637c13bd4db6c871aac1adfa813d49801ed2c68d7763aad67` | nothing |
| `internal/gnovm/testdata/gnomem/v2/grammar.golden` | `internal/gnovm/testdata/gnomem/v2/grammar.golden` | carried | 7,460 | 0.0% | 0 | `014ce79ebdc655d2221e2348a4847246523b6f98bde118536683a6967fd057f1` | nothing |
| `internal/gnovm/testdata/gnomem/v2/story.golden` | `internal/gnovm/testdata/gnomem/v2/story.golden` | carried | 13,163 | 0.0% | 0 | `7435844f804724ca3553488f57b91b088ed4224e67e66622af4ab117c4dc6e79` | nothing |
| `internal/gnovm/testdata/memupgrade/empty-layout2/index-cad6e3aebb324d04.db` | `internal/gnovm/testdata/memupgrade/empty-layout2/index-cad6e3aebb324d04.db` | carried | 36,864 | n/a (binary) | n/a (binary) | `1ecd8546d1538e0951ac2c30888d2a30412794b862006474e816709ae904affd` | nothing |
| `internal/gnovm/testdata/memupgrade/empty-layout2/journal.jsonl` | `internal/gnovm/testdata/memupgrade/empty-layout2/journal.jsonl` | carried | 189 | 0.0% | 0 | `6574c208989e6f7477c3ad337ff7db147e7add9ec7f5e79df51ea71bc0f7b4a1` | nothing |
| `internal/gnovm/testdata/memupgrade/empty-layout2/store-cad6e3aebb324d04/mem.db/000002.ldb` | `internal/gnovm/testdata/memupgrade/empty-layout2/store-cad6e3aebb324d04/mem.db/000002.ldb` | carried | 2,492 | n/a (binary) | n/a (binary) | `59c427c19497a9a9dbb928392940e51f3d519e83bc4b41bf9250952762d01b2f` | nothing |
| `internal/gnovm/testdata/memupgrade/empty-layout2/store-cad6e3aebb324d04/mem.db/CURRENT` | `internal/gnovm/testdata/memupgrade/empty-layout2/store-cad6e3aebb324d04/mem.db/CURRENT` | carried | 16 | 0.0% | 0 | `01bd196d6a114691ec642082ebf6591765c0168d4098a0cd834869bd11c8b87d` | nothing |
| `internal/gnovm/testdata/memupgrade/empty-layout2/store-cad6e3aebb324d04/mem.db/CURRENT.bak` | `internal/gnovm/testdata/memupgrade/empty-layout2/store-cad6e3aebb324d04/mem.db/CURRENT.bak` | carried | 16 | 0.0% | 0 | `c3225bb8babf9823a2daf2bccae0cafc5d3e0857c5f24187dc004f1b2560b4db` | nothing |
| `internal/gnovm/testdata/memupgrade/empty-layout2/store-cad6e3aebb324d04/mem.db/LOCK` | `internal/gnovm/testdata/memupgrade/empty-layout2/store-cad6e3aebb324d04/mem.db/LOCK` | carried | 0 | 0.0% | 0 | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` | nothing |
| `internal/gnovm/testdata/memupgrade/empty-layout2/store-cad6e3aebb324d04/mem.db/LOG` | `internal/gnovm/testdata/memupgrade/empty-layout2/store-cad6e3aebb324d04/mem.db/LOG` | carried | 5,841 | 0.0% | 0 | `fa7ed53dd2bddc1f6623527f003400880f693b830d87699a9d599e0b7013fd17` | nothing |
| `internal/gnovm/testdata/memupgrade/empty-layout2/store-cad6e3aebb324d04/mem.db/MANIFEST-000020` | `internal/gnovm/testdata/memupgrade/empty-layout2/store-cad6e3aebb324d04/mem.db/MANIFEST-000020` | carried | 120 | n/a (binary) | n/a (binary) | `e7f9ab0811eac33f872e1bb14fe1149af42b9111d1a75caecfe1d0092ce569c0` | nothing |
| `internal/gnovm/testdata/memupgrade/empty-layout2/vectors-cad6e3aebb324d04.db` | `internal/gnovm/testdata/memupgrade/empty-layout2/vectors-cad6e3aebb324d04.db` | carried | 16,384 | n/a (binary) | n/a (binary) | `f46df6ab7b2e596d9935e3861f3e25617461b39a74a133f068743a532db3108f` | nothing |
| `internal/gnovm/testdata/memupgrade/goals-layout2/index-768ac3294ec6f9f9.db` | `internal/gnovm/testdata/memupgrade/goals-layout2/index-768ac3294ec6f9f9.db` | carried | 36,864 | n/a (binary) | n/a (binary) | `816328fbdcc3c260fd390bf973faeeedf6018bac7869fd769e6820d19f71c342` | nothing |
| `internal/gnovm/testdata/memupgrade/goals-layout2/journal.jsonl` | `internal/gnovm/testdata/memupgrade/goals-layout2/journal.jsonl` | carried | 1,099 | 0.0% | 0 | `b459b5f743c73a3e48a9f18a4063b241ea0532bd2e2394d33f8aa628964ac386` | nothing |
| `internal/gnovm/testdata/memupgrade/goals-layout2/store-768ac3294ec6f9f9/mem.db/000002.ldb` | `internal/gnovm/testdata/memupgrade/goals-layout2/store-768ac3294ec6f9f9/mem.db/000002.ldb` | carried | 13,244 | n/a (binary) | n/a (binary) | `eb080b2672fc2dfb72c5377d81bd3bc1a6f9e077cbb8ea0a2fb709cfa7e6ea98` | nothing |
| `internal/gnovm/testdata/memupgrade/goals-layout2/store-768ac3294ec6f9f9/mem.db/CURRENT` | `internal/gnovm/testdata/memupgrade/goals-layout2/store-768ac3294ec6f9f9/mem.db/CURRENT` | carried | 16 | 0.0% | 0 | `4166bf17b2da789b0d0cc5c74203041d98005f5d4ef88c27e8281e00148cd611` | nothing |
| `internal/gnovm/testdata/memupgrade/goals-layout2/store-768ac3294ec6f9f9/mem.db/CURRENT.bak` | `internal/gnovm/testdata/memupgrade/goals-layout2/store-768ac3294ec6f9f9/mem.db/CURRENT.bak` | carried | 16 | 0.0% | 0 | `0861415cada612ea5834d56e2cf1055d3e63979b69eb71d32ae9ae394d8306cd` | nothing |
| `internal/gnovm/testdata/memupgrade/goals-layout2/store-768ac3294ec6f9f9/mem.db/LOCK` | `internal/gnovm/testdata/memupgrade/goals-layout2/store-768ac3294ec6f9f9/mem.db/LOCK` | carried | 0 | 0.0% | 0 | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` | nothing |
| `internal/gnovm/testdata/memupgrade/goals-layout2/store-768ac3294ec6f9f9/mem.db/LOG` | `internal/gnovm/testdata/memupgrade/goals-layout2/store-768ac3294ec6f9f9/mem.db/LOG` | carried | 1,701 | 0.0% | 0 | `59295fd2f5d0acde9fd350c271121d7b1df2c4e5f53373d3fd958f0f437a2a19` | nothing |
| `internal/gnovm/testdata/memupgrade/goals-layout2/store-768ac3294ec6f9f9/mem.db/MANIFEST-000006` | `internal/gnovm/testdata/memupgrade/goals-layout2/store-768ac3294ec6f9f9/mem.db/MANIFEST-000006` | carried | 120 | n/a (binary) | n/a (binary) | `5c5e8669392179eb7c183d16b7bcc2cf196eea5e5824d397027173b540831fa8` | nothing |
| `internal/gnovm/testdata/scenarios/ask-before-a-todo.txtar` | `internal/gnovm/testdata/scenarios/ask-before-a-todo.txtar` | carried | 1,257 | 0.0% | 0 | `05a97f45b71060f9085c4f33a5aafdf1ae6781381528ca9f4f3de1b920ae96b7` | nothing |
| `internal/gnovm/testdata/scenarios/asked-what-was-done.txtar` | `internal/gnovm/testdata/scenarios/asked-what-was-done.txtar` | carried | 2,020 | 5.0% | 0 | `21334f797dddd556e6ddb540253dd42e7d858aee2834290504442a9a31ff95a4` | nothing |
| `internal/gnovm/testdata/scenarios/audit-gno-new-project.txtar` | `internal/gnovm/testdata/scenarios/audit-gno-new-project.txtar` | carried | 3,145 | 0.0% | 0 | `327d755372c0938af07a3c081f607fd889713c7a43e8d964dbf2a6ca7e761f50` | nothing |
| `internal/gnovm/testdata/scenarios/cancel-and-revive.txtar` | `internal/gnovm/testdata/scenarios/cancel-and-revive.txtar` | carried | 879 | 0.0% | 0 | `4583f66fe29336594596e5af0abd2485ebedff3f13f1f788e9108697f83b4906` | nothing |
| `internal/gnovm/testdata/scenarios/checklist-calc.txtar` | `internal/gnovm/testdata/scenarios/checklist-calc.txtar` | carried | 1,076 | 0.0% | 0 | `ed7181b93bd3356c0bca9979130c7a0502cf1c8e194bb98276d3446f7a06d6fe` | nothing |
| `internal/gnovm/testdata/scenarios/compute-backoff-after-hi.txtar` | `internal/gnovm/testdata/scenarios/compute-backoff-after-hi.txtar` | carried | 744 | 0.0% | 0 | `396e942fac354b2a33edcbb83937ce1df15e2bf409a46110a757985b6d7ef6cb` | nothing |
| `internal/gnovm/testdata/scenarios/compute-backoff.txtar` | `internal/gnovm/testdata/scenarios/compute-backoff.txtar` | carried | 739 | 0.0% | 0 | `f47598cc613d6de208332edc8a4edb34dd0ef0110f555b739709b99f78840ff0` | nothing |
| `internal/gnovm/testdata/scenarios/compute-primes.txtar` | `internal/gnovm/testdata/scenarios/compute-primes.txtar` | carried | 243 | 0.0% | 0 | `ea7283b111a1d71b6cf699a084b8cba08722a76e05a704ea9255651e283cc56a` | nothing |
| `internal/gnovm/testdata/scenarios/convention-applied-later.txtar` | `internal/gnovm/testdata/scenarios/convention-applied-later.txtar` | carried | 1,451 | 0.0% | 0 | `345c735a901cd3f980764570ebac7831e7796dd10ddccf39ab1ef01ed1f65bb0` | nothing |
| `internal/gnovm/testdata/scenarios/do-the-todo-asked-for.txtar` | `internal/gnovm/testdata/scenarios/do-the-todo-asked-for.txtar` | carried | 1,287 | 0.0% | 0 | `f8311966aa225bcf49cbde0800536b43545958c427e54e0c8ac7daf37519f9d2` | nothing |
| `internal/gnovm/testdata/scenarios/drop-on-a-change-of-mind.txtar` | `internal/gnovm/testdata/scenarios/drop-on-a-change-of-mind.txtar` | carried | 1,165 | 0.0% | 0 | `54e1997c0e0aae3cbcc3271737981f96c5f0d8201541fb7ab5d961b447b3cd7b` | nothing |
| `internal/gnovm/testdata/scenarios/eight-todos-one-session.txtar` | `internal/gnovm/testdata/scenarios/eight-todos-one-session.txtar` | carried | 1,227 | 0.0% | 0 | `51befe0900a979c2f474fb553ced54f226d83b91e07440432ddbf779b1b56f77` | nothing |
| `internal/gnovm/testdata/scenarios/fact-said-inside-a-plan.txtar` | `internal/gnovm/testdata/scenarios/fact-said-inside-a-plan.txtar` | carried | 1,541 | 0.0% | 0 | `ed73f804eca91a6225aadcf6a15a885eefb3e2b1e79e81099fdde667be195b15` | nothing |
| `internal/gnovm/testdata/scenarios/facts-said-in-passing.txtar` | `internal/gnovm/testdata/scenarios/facts-said-in-passing.txtar` | carried | 578 | 0.0% | 0 | `c83fec411a36915dabe73adc9b380f59332ca7b72c23f77138df766aeff340a0` | nothing |
| `internal/gnovm/testdata/scenarios/findings-for-follow-up.txtar` | `internal/gnovm/testdata/scenarios/findings-for-follow-up.txtar` | carried | 4,495 | 0.0% | 0 | `63c117dda7e2ac359756ef1e443c2a23323a291a7cdd9da76d9a1d6136baf1c5` | nothing |
| `internal/gnovm/testdata/scenarios/forget-a-secret.txtar` | `internal/gnovm/testdata/scenarios/forget-a-secret.txtar` | carried | 873 | 0.0% | 0 | `c8d7a6d134fbc2d6eec10f776233b39b795ffd43ea7f95430d8fd1792dde0ecd` | nothing |
| `internal/gnovm/testdata/scenarios/goal-claimed-done-not-done.txtar` | `internal/gnovm/testdata/scenarios/goal-claimed-done-not-done.txtar` | carried | 853 | 0.0% | 0 | `b859a5aa1ae08fc618f51bd05e90be9fcabca4de395d94842590cea9e7d6c3a7` | nothing |
| `internal/gnovm/testdata/scenarios/goal-done-elsewhere.txtar` | `internal/gnovm/testdata/scenarios/goal-done-elsewhere.txtar` | carried | 842 | 0.0% | 0 | `66fdc8201aee68515def1403c46028d6971d99e0b78e250c7c96ff13f43f0739` | nothing |
| `internal/gnovm/testdata/scenarios/goal-judged-met.txtar` | `internal/gnovm/testdata/scenarios/goal-judged-met.txtar` | carried | 703 | 0.0% | 0 | `fbadaeb857b3d9ade45982f1e6e23418c185bb044b9454d7e8f7e4358568839d` | nothing |
| `internal/gnovm/testdata/scenarios/goal-replan.txtar` | `internal/gnovm/testdata/scenarios/goal-replan.txtar` | carried | 1,319 | 0.0% | 0 | `e66778a581140404c6b82973cc6c7ff5dc0cb2ce8034b9a42739d52c424682cd` | nothing |
| `internal/gnovm/testdata/scenarios/goal-until-green.txtar` | `internal/gnovm/testdata/scenarios/goal-until-green.txtar` | carried | 1,687 | 0.0% | 0 | `b444df44b6d91dc6e0920d8f5b3cbc874aade13dd25f14d42e9a8c38a3647388` | nothing |
| `internal/gnovm/testdata/scenarios/instruction-for-a-later-todo.txtar` | `internal/gnovm/testdata/scenarios/instruction-for-a-later-todo.txtar` | carried | 1,637 | 0.0% | 0 | `80f7e8301a48f351986d108ff8c665894908f92ce219fb95b195a42b4cf4c880` | nothing |
| `internal/gnovm/testdata/scenarios/long-goal-sessions.txtar` | `internal/gnovm/testdata/scenarios/long-goal-sessions.txtar` | carried | 1,234 | 0.0% | 0 | `e10f6d4fe72fcfc06041e22fd3185535ae8e81b280280e8608c67e9072347aef` | nothing |
| `internal/gnovm/testdata/scenarios/mem-stays-out-of-the-way.txtar` | `internal/gnovm/testdata/scenarios/mem-stays-out-of-the-way.txtar` | carried | 629 | 0.0% | 0 | `0fbcd569a8fb0c91021cc5b0950345ed507035d60e9ebc47ad444768e673130a` | nothing |
| `internal/gnovm/testdata/scenarios/memory-across-sessions.txtar` | `internal/gnovm/testdata/scenarios/memory-across-sessions.txtar` | carried | 533 | 0.0% | 0 | `f9d8923549cb04ebad06b0354350a3e78c705d7d88970e371a60b0e5f8f2be87` | nothing |
| `internal/gnovm/testdata/scenarios/memory-update.txtar` | `internal/gnovm/testdata/scenarios/memory-update.txtar` | carried | 563 | 0.0% | 0 | `8ba7f1707be429cea04e474628f4da2d34477db3bb28dfe232e4054af33d5655` | nothing |
| `internal/gnovm/testdata/scenarios/never-keep-a-secret.txtar` | `internal/gnovm/testdata/scenarios/never-keep-a-secret.txtar` | carried | 705 | 0.0% | 0 | `749084d9dc62e69c5e2a64602950a9090d4d63c8f54c48699efbe5d5722b07ba` | nothing |
| `internal/gnovm/testdata/scenarios/noticed-for-later.txtar` | `internal/gnovm/testdata/scenarios/noticed-for-later.txtar` | carried | 1,445 | 0.0% | 0 | `07802ef72dc33075e7893c8ddb7b1d5bae43cdf5dd533edd096c9a7418cd4843` | nothing |
| `internal/gnovm/testdata/scenarios/one-at-a-time-across-turns.txtar` | `internal/gnovm/testdata/scenarios/one-at-a-time-across-turns.txtar` | carried | 1,379 | 0.0% | 0 | `42a19235a7ac66671a1717995a36d1c95b45e004f72db3011c5a4f321d609c41` | nothing |
| `internal/gnovm/testdata/scenarios/one-off-question-leaves-mem-alone.txtar` | `internal/gnovm/testdata/scenarios/one-off-question-leaves-mem-alone.txtar` | carried | 738 | 0.0% | 0 | `b5ae6d5df6b7ef13438e624ee9dc2f075d9864d94acb0725a480c5b49a8011a1` | nothing |
| `internal/gnovm/testdata/scenarios/peers-algos-control-calc.txtar` | `internal/gnovm/testdata/scenarios/peers-algos-control-calc.txtar` | carried | 14,999 | 0.0% | 0 | `0a089286b6ccc45b5f0f02ea902c81285f0f2ae8f693fae2d1ac5d3af317e6f2` | nothing |
| `internal/gnovm/testdata/scenarios/peers-algos-control-data.txtar` | `internal/gnovm/testdata/scenarios/peers-algos-control-data.txtar` | carried | 15,013 | 0.0% | 0 | `9b0d50b19274f3ed6d06d3f60ef2cb2b6a0eb8d3121c329207a55cb137277366` | nothing |
| `internal/gnovm/testdata/scenarios/peers-algos-control-text.txtar` | `internal/gnovm/testdata/scenarios/peers-algos-control-text.txtar` | carried | 15,021 | 0.0% | 0 | `0fc2ab9ad6a92cf3528c52490293f292f82f072ca75fd15cecfbde35d6107b90` | nothing |
| `internal/gnovm/testdata/scenarios/peers-algos.txtar` | `internal/gnovm/testdata/scenarios/peers-algos.txtar` | carried | 16,477 | 0.0% | 0 | `1ffa9d1b9c582df3389a7daf27b1d7498b30ac03b629b9abdc415be7f788514d` | nothing |
| `internal/gnovm/testdata/scenarios/peers-control-circle.txtar` | `internal/gnovm/testdata/scenarios/peers-control-circle.txtar` | carried | 4,769 | 0.0% | 0 | `c245fe267e3400b6e214af02cb044e44471357ea4035fb9e34e4c0274d3c1a6b` | nothing |
| `internal/gnovm/testdata/scenarios/peers-control-square.txtar` | `internal/gnovm/testdata/scenarios/peers-control-square.txtar` | carried | 4,769 | 0.0% | 0 | `4954575d2bc77cfb64a17f2146eddc0a97be56e0bfbadff9ebbaae64a58aa9c6` | nothing |
| `internal/gnovm/testdata/scenarios/peers-control-triangle.txtar` | `internal/gnovm/testdata/scenarios/peers-control-triangle.txtar` | carried | 4,793 | 0.0% | 0 | `222b3f8b1d7ee2b8773da55d93d2700e1e9eaffcd6f84c447e72a223c7d9f108` | nothing |
| `internal/gnovm/testdata/scenarios/peers-geom.txtar` | `internal/gnovm/testdata/scenarios/peers-geom.txtar` | carried | 5,906 | 0.0% | 0 | `68e3fa7c8902508c5e7704173ad71ca9ab5368dae5e8fa4d4a99434d1a96a22a` | nothing |
| `internal/gnovm/testdata/scenarios/peers-shapes-control-circle.txtar` | `internal/gnovm/testdata/scenarios/peers-shapes-control-circle.txtar` | carried | 9,934 | 0.0% | 0 | `3914da5aab25bbfb375c5a999b42e5df28f58a360c0bc500ad0e8b2405aa17b7` | nothing |
| `internal/gnovm/testdata/scenarios/peers-shapes-control-square.txtar` | `internal/gnovm/testdata/scenarios/peers-shapes-control-square.txtar` | carried | 9,934 | 0.0% | 0 | `2f0e58c066fdc5d4009612ca45709b4cc6f07ad28c5fc185d4d1f6138623e6ee` | nothing |
| `internal/gnovm/testdata/scenarios/peers-shapes-control-triangle.txtar` | `internal/gnovm/testdata/scenarios/peers-shapes-control-triangle.txtar` | carried | 9,974 | 0.0% | 0 | `4a5efd33e84779976c674c8a4e76cbfe14d221e70b1b15701cc846beb2abfef2` | nothing |
| `internal/gnovm/testdata/scenarios/peers-shapes.txtar` | `internal/gnovm/testdata/scenarios/peers-shapes.txtar` | carried | 12,046 | 0.0% | 0 | `d62b0e04bed766098919c2c794fac54e7a4f9700ef69ff559cb08f39a34889e6` | nothing |
| `internal/gnovm/testdata/scenarios/plan-inside-a-todo.txtar` | `internal/gnovm/testdata/scenarios/plan-inside-a-todo.txtar` | carried | 1,987 | 0.0% | 0 | `9d997e7f5dee5f7ac8a3fe2768d3e2c4a7b57efab892c7a7ba46ed13fb06bf4f` | nothing |
| `internal/gnovm/testdata/scenarios/prompt-arrives-inside-a-todo.txtar` | `internal/gnovm/testdata/scenarios/prompt-arrives-inside-a-todo.txtar` | carried | 1,809 | 0.0% | 0 | `b3e5f3a7455f5c5b68cd37eb4595a1a2f3590bb2d909d5be7e42c750f801fdd5` | nothing |
| `internal/gnovm/testdata/scenarios/recall-beyond-the-block.txtar` | `internal/gnovm/testdata/scenarios/recall-beyond-the-block.txtar` | carried | 1,799 | 0.0% | 0 | `9a3c8fd1c206f18255affe0ea3b3ec8228958e2d8e36c3f6a1d3ba64f1e0694e` | nothing |
| `internal/gnovm/testdata/scenarios/recall-by-meaning.txtar` | `internal/gnovm/testdata/scenarios/recall-by-meaning.txtar` | carried | 3,019 | 0.0% | 0 | `cbf2fcf973b43e9cfe579e9ed74ce2065b8d515cee8ff13f5809ea93fcc4fbf1` | nothing |
| `internal/gnovm/testdata/scenarios/recall-paraphrased.txtar` | `internal/gnovm/testdata/scenarios/recall-paraphrased.txtar` | carried | 523 | 0.0% | 0 | `28748b4148bbc3959b0e9a46725445a552e29b328bb281f257dcf0a71263f121` | nothing |
| `internal/gnovm/testdata/scenarios/release-signoff-closes.txtar` | `internal/gnovm/testdata/scenarios/release-signoff-closes.txtar` | carried | 662 | 0.0% | 0 | `7eba21b4b946530c06fe70ea6c52fa1ccfa7e52465589433e8976530c0e5e342` | nothing |
| `internal/gnovm/testdata/scenarios/release-waits-for-signoff.txtar` | `internal/gnovm/testdata/scenarios/release-waits-for-signoff.txtar` | carried | 1,061 | 0.0% | 0 | `df0fb9a04dc62d4ed664b5fd73e8728482f67edbca8e551c87158ee5c7453bd0` | nothing |
| `internal/gnovm/testdata/scenarios/remind-next-time.txtar` | `internal/gnovm/testdata/scenarios/remind-next-time.txtar` | carried | 438 | 0.0% | 0 | `8a478f3de4bbef38e365837ba50e1d98213a772a14e1783b09ff85c51a13d05a` | nothing |
| `internal/gnovm/testdata/scenarios/remind-on-a-date.txtar` | `internal/gnovm/testdata/scenarios/remind-on-a-date.txtar` | carried | 605 | 0.0% | 0 | `eb4d64b2dfa0d56d9ba5f5197661f26fd8d8e02a12f2270a6bf1ca4f72b33f88` | nothing |
| `internal/gnovm/testdata/scenarios/reminder-beside-unrelated-work.txtar` | `internal/gnovm/testdata/scenarios/reminder-beside-unrelated-work.txtar` | carried | 645 | 0.0% | 0 | `b045ca7c466515caa08405612ae8460bd553977cc5de4b0e29ad7aad8ca36f44` | nothing |
| `internal/gnovm/testdata/scenarios/rename-project.txtar` | `internal/gnovm/testdata/scenarios/rename-project.txtar` | carried | 2,232 | 0.0% | 0 | `1b9a9c1cb7296e03a6982f705fcf3d9b20f1422a5f1ef2e52e702ca75efcb23a` | nothing |
| `internal/gnovm/testdata/scenarios/reopen-a-false-done.txtar` | `internal/gnovm/testdata/scenarios/reopen-a-false-done.txtar` | carried | 1,536 | 0.0% | 0 | `16ef06639ff49e0ec5e6eaf0b74ca8d54a742a9416fa14f799051fc747062486` | nothing |
| `internal/gnovm/testdata/scenarios/replan-a-step-that-cannot-work.txtar` | `internal/gnovm/testdata/scenarios/replan-a-step-that-cannot-work.txtar` | carried | 899 | 0.0% | 0 | `2e0b6443d4f038992bf1a506c2d3ed5cf2c967d4b49d54177fa74c0e6b7c75c8` | nothing |
| `internal/gnovm/testdata/scenarios/research-carries-over.txtar` | `internal/gnovm/testdata/scenarios/research-carries-over.txtar` | carried | 1,328 | 0.0% | 0 | `4ed0926f4616f41486d38f4189abb1281983f0ae9edae42d15d96c9e6f2ec330` | nothing |
| `internal/gnovm/testdata/scenarios/resume-checklist.txtar` | `internal/gnovm/testdata/scenarios/resume-checklist.txtar` | carried | 850 | 0.0% | 0 | `e2cb00d28294269428fd46c65df555029b354658215040ff9a67555900d5c0dd` | nothing |
| `internal/gnovm/testdata/scenarios/review-notes-then-fix.txtar` | `internal/gnovm/testdata/scenarios/review-notes-then-fix.txtar` | carried | 2,339 | 3.4% | 0 | `b4a767052b41d69a13059d8bec4748ccf6669dc66ed6cfc63a0c94092141543f` | nothing |
| `internal/gnovm/testdata/scenarios/rule-from-the-user.txtar` | `internal/gnovm/testdata/scenarios/rule-from-the-user.txtar` | carried | 430 | 0.0% | 0 | `d28fd4950f7a9db2539ba5d6c4d6f2401bb2f9c31fa5ab078546c9d9721dc6b6` | nothing |
| `internal/gnovm/testdata/scenarios/side-fix-found-on-the-way.txtar` | `internal/gnovm/testdata/scenarios/side-fix-found-on-the-way.txtar` | carried | 1,192 | 0.0% | 0 | `ef3b9b1962177daba8feeaf2414d960a3b1b20e4702c23aa1204d1cbdeb45fc7` | nothing |
| `internal/gnovm/testdata/scenarios/slow-tests-findings-then-fix.txtar` | `internal/gnovm/testdata/scenarios/slow-tests-findings-then-fix.txtar` | carried | 2,349 | 0.0% | 0 | `e8d6877bf2999a541bc1d6cd2872c5e0229c933d2e578eaac0a85e089fa8617c` | nothing |
| `internal/gnovm/testdata/scenarios/thought-becomes-a-todo.txtar` | `internal/gnovm/testdata/scenarios/thought-becomes-a-todo.txtar` | carried | 1,415 | 0.0% | 0 | `a2355bd4c79295b220885dc249065c917a5b5adf8d8ca92e547c2ac2f024b1ea` | nothing |
| `internal/gnovm/testdata/scenarios/todo-list-next-day.txtar` | `internal/gnovm/testdata/scenarios/todo-list-next-day.txtar` | carried | 583 | 0.0% | 0 | `fc7b80a0dca7e093323ac6c321e1089c32938509068af59f6a2f2b2cdefb0c98` | nothing |
| `internal/gnovm/testdata/scenarios/todo-list-over-days.txtar` | `internal/gnovm/testdata/scenarios/todo-list-over-days.txtar` | carried | 567 | 0.0% | 0 | `8869426c8eab6d8a1bbf5d2eb827e3f2daf8309d8c8450491b589af3be1c5798` | nothing |
| `internal/gnovm/testdata/scenarios/two-tasks-interleaved.txtar` | `internal/gnovm/testdata/scenarios/two-tasks-interleaved.txtar` | carried | 676 | 0.0% | 0 | `72918decefbb0ed60a187dfd5f3817a8e439500f03d278738ccb5c95d478c101` | nothing |
| `internal/gnovm/testdata/scenarios/unknown-fact-honest.txtar` | `internal/gnovm/testdata/scenarios/unknown-fact-honest.txtar` | carried | 495 | 0.0% | 0 | `7bbc1d2b2a015338c7192570576bec75b70f58d44da3589961665df1a2d1df1e` | nothing |
| `internal/gnovm/testdata/scenarios/user-can-see-mem.txtar` | `internal/gnovm/testdata/scenarios/user-can-see-mem.txtar` | carried | 388 | 0.0% | 0 | `a119a1b936c41b88b1ef5a5a4c1bd1f6514006a3ea9817b8c4cb4d9c5cdfc0ff` | nothing |
| `internal/gnovm/testdata/scenarios/user-words-inside-a-frame.txtar` | `internal/gnovm/testdata/scenarios/user-words-inside-a-frame.txtar` | carried | 1,747 | 0.0% | 0 | `4f262d894f1f0fba221b278f72f2ed4b737ab402ca51aa12e936d65aa35cf9bf` | nothing |
| `internal/gnovm/testdata/scenarios/wait-inside-a-plan.txtar` | `internal/gnovm/testdata/scenarios/wait-inside-a-plan.txtar` | carried | 1,185 | 0.0% | 0 | `f4b54dbc66354f70a06879253581222eee32fad10e5ec932a08184f349ce0df4` | nothing |
| `internal/gnovm/testdata/scenarios/wait-on-a-merge.txtar` | `internal/gnovm/testdata/scenarios/wait-on-a-merge.txtar` | carried | 422 | 0.0% | 0 | `3a63f6d095126c9161f84e4bfb85faaac9fe90ff41816db3227dd372ac49edb3` | nothing |
| `internal/gnovm/testdata/scenarios/where-are-we.txtar` | `internal/gnovm/testdata/scenarios/where-are-we.txtar` | carried | 783 | 0.0% | 0 | `a1cb6842563d19a09b37e88c295110c67e2d5b21c92c730e1ff01bc2d8c488fa` | nothing |
| `internal/gnovm/trips_test.go` | `internal/gnovm/trips_test.go` | carried | 8,342 | 0.8% | 0 | `e9c24ded92c23245163b424135f32df33a41245bb69323bff41d703459c22c6c` | nothing |
| `internal/gnovm/turns.go` | `internal/gnovm/turns.go` | carried with changes | 3,496 | 7.5% | 0 | `8d9fa11d252a9ec29615e307af9fb83f6a2e029648eb6666d68ef71a5be76db6` | import path, 1 line: the earlier harness's `internal/lock` is now `internal/filelock`, imported under the name `lock` so no call changed |
| `internal/gnovm/vm.go` | `internal/gnovm/vm.go` | carried | 14,577 | 0.0% | 0 | `13b3a671697d0790613cedbdc4c14314478a1eed64c0dfc43c26e092db608f9b` | nothing |
| `internal/gnovm/wire.go` | `internal/gnovm/wire.go` | carried | 7,493 | 7.7% | 0 | `b7b438e9a65672a18c4641eb267ccfd74014ba730f56be16e1d6224cc65e546b` | nothing |
| `internal/gnovm/worker.go` | `internal/gnovm/worker.go` | carried with changes | 8,487 | 0.0% | 0 | `ce552860ed9ea7574169f8516a946a8c4fa16e1bd45ca3ec376ab1efc3abea6f` | import path, 1 line: the earlier harness's `internal/version` is now `internal/buildinfo`, imported under the name `version` so no call changed |
| `internal/gnovm/worker_test.go` | `internal/gnovm/worker_test.go` | carried | 10,112 | 1.4% | 0 | `ad98650b6007ee774b2df2b49ff079dd13b02aebf1be68a78f14eeb93462cf7a` | nothing |
| `internal/gnovm/worker_unix.go` | `internal/gnovm/worker_unix.go` | carried | 2,508 | 0.0% | 0 | `82e226f01c7b416182b27b7aa1d1a4239b77fa16d1deff93c8804457abf3f514` | nothing |
| `internal/gnovm/worker_windows.go` | `internal/gnovm/worker_windows.go` | carried | 814 | 0.0% | 0 | `ce198886ee1c89164544ec218c6567573722737b160c2a310cdbede8dc5330a6` | nothing |
| `internal/swarm/cmd/swarmdemo/halt_other.go` | `internal/swarm/cmd/swarmdemo/halt_other.go` | carried | 214 | 0.0% | 0 | `751b7941956dc4193fab6729b870bc71f27d88c21f50285b8a5afb065840c6a3` | nothing |
| `internal/swarm/cmd/swarmdemo/halt_unix.go` | `internal/swarm/cmd/swarmdemo/halt_unix.go` | carried | 323 | 0.0% | 0 | `1c9b9e104c20e38093ef1e97ad2289e7253b0ea44e316c5f9c64f20f19cd63bd` | nothing |
| `internal/swarm/cmd/swarmdemo/main.go` | `internal/swarm/cmd/swarmdemo/main.go` | carried with changes | 40,578 | 0.6% | 0 | `a655899f6a63a9857fc991729b00c1dd98fed1f8f3115f608e5c3e8780369c1a` | import path, 1 line |

<!-- carried-table:end -->

## The changes to carried files

Ten files were changed, one line in each, and no line was added or removed.
The table says which; this is why.

- **The module path.** The earlier tree's module was still named
  `github.com/charmbracelet/crush`; this one is `github.com/gnosh-ai/gnosh`.
  Two lines name a package of the tree by its full path and follow the
  rename: the import of `internal/gnochain` in
  `internal/swarm/cmd/swarmdemo/main.go`, and the path under which
  `internal/gnochain/msg.go` registers its own package with amino (amino
  panics at start-up when a type's package path is not the one registered).
- **The file-lock helper.** Six files of `internal/gnovm` imported the
  earlier harness's `internal/lock`, which was not staged. They now import
  `internal/filelock` (below) under the name `lock`, so every call
  (`lock.TryFile`, `lock.File`, `lock.ErrContended`) stands as it was.
- **The build id.** `internal/gnovm/worker.go` imported the earlier
  harness's `internal/version` for `version.BuildID`. It now imports
  `internal/buildinfo` (below) under the name `version`, so the expression
  stands as it was. The comment in `internal/gnovm/realworker_test.go` that
  tells how to give two binaries the same build id names the linker flag by
  its new path.

Words about the earlier harness that were left as they are, because they are
history, or names the code still reads, or not the porter's to change:

- `internal/gnovm/LICENSE.md`, under "Known license interactions", says that
  most of gnosh is derived from Charm's Crush and is under the Functional
  Source License. That is no longer true of this repository. It is a licence
  file and was carried unchanged; the correction is its Licensor's to make.
- `internal/gnovm/DESIGN.md` and `GNOMEM.md` describe the subsystem as it
  was built inside the earlier harness (its tools, its `/mem` and `/goal`
  commands, a context file named `CRUSH.md`, `testenv`).
- `internal/gnovm/scenario_test.go` copies the earlier harness's
  configuration files (`crushrc`, `crush.json`, under `config/crush` and
  `data/crush`) for the gnosh binary it drives. The test runs only when it
  is given such a binary.
- `internal/gnovm/client.go` drops `GNOSH_PROFILE`, `KLASH_PROFILE` and
  `CRUSH_PROFILE` from a worker's environment.
- `klashlibs` and the import path `klash/gnomem/v2` keep an earlier name of
  gnosh: mem's journals on disk import the library by that path.

## The tools, carried with larger changes

`internal/gnovm/tools/` is the four tools a model calls to use the GnoVM
subsystem: `gnovm_run`, `gnovm_new`, `gnovm_list` and `gnovm_drop`.
`gnovm_run` is how a model reaches `mem`. Its seven staged files are the
earlier gnosh's own work, as the scan found (no run of lines shared with
Charm's code in any of them). Five of the seven were ADAPT in the scan: they
call into the earlier harness, which is not here. The porter read those
seven staged files, the inventory's record for the package (words and
numbers) and its lists of the symbols they used (names only), and nothing
else of the earlier tree: not the code those calls led to, and not the
library the earlier harness's tool interface came from. Each call into the
earlier harness was replaced by this harness's way of doing the same thing,
from `internal/core` and from `spec/gnosh/machines.md` and
`spec/gnosh/integration-contract.md`.

So these files are carried with changes, and the changes are larger than an
import path. Every file keeps its path and its name. What a model or a
person reads of the tools was kept word for word: the tools' names, their
descriptions and their arguments' names, types and descriptions, every
result and every error.

<!-- carried-tools:begin (written by tools/audit/carried.py; do not edit by hand) -->

| file | made from the staged | the scan's verdict | symbols of the earlier harness it used | bytes | line share | longest run | SHA-256 of the staged original | what it is now |
|---|---|---|---:|---:|---:|---:|---|---|
| `internal/gnovm/tools/closecheck_test.go` | `internal/gnovm/tools/closecheck_test.go` | CARRY | 0 | 5,714 | 0.0% | 0 | `3ac02e4070c3bcb544e598f79d87d37f37487b3f7be7f41d7a41e0a271d7ed2c` | the same test, on this harness's tool interface |
| `internal/gnovm/tools/docs_test.go` | `internal/gnovm/tools/docs_test.go` | ADAPT | 1 | 32,330 | 1.2% | 0 | `81eef8d1b8885aff0281ff77897b102ae2a0f9da21fca1d7a946bc7e1d0c4635` | the same tests for GNOMEM.md; those of the earlier README and of the earlier harness's goal file are left out |
| `internal/gnovm/tools/memtodos.go` | `internal/gnovm/tools/memtodos.go` | ADAPT | 8 | 2,618 | 0.0% | 0 | `fb57a815625b4c138ba695831f1bb82afe5e1dd58d1f0d5916a8ba7b5d2742ed` | the same mapping of a goal to a task list, onto a task-list type of its own |
| `internal/gnovm/tools/memtodos_test.go` | `internal/gnovm/tools/memtodos_test.go` | ADAPT | 11 | 2,557 | 0.0% | 0 | `6005bda8ef20fb1ebdec2f94545bcbb6777a8bb2c7c58a1327df0b20fdada54a` | the same test, on that type |
| `internal/gnovm/tools/namer_test.go` | `internal/gnovm/tools/namer_test.go` | CARRY | 0 | 1,724 | 0.0% | 0 | `1615ddf3667a042b56b172730584f3f5f1611acc3b0b645aa6b437e462581359` | the same test |
| `internal/gnovm/tools/tools.go` | `internal/gnovm/tools/tools.go` | ADAPT | 11 | 28,001 | 1.7% | 0 | `d4383a347d7da6d8db3a30b5ec0ba7b271f2fc141a8a43d3ae80f991bbef73ed` | the four tools as core.Tool values; the permission prompt is gone from it |
| `internal/gnovm/tools/tools_test.go` | `internal/gnovm/tools/tools_test.go` | ADAPT | 2 | 4,986 | 3.2% | 0 | `ec451fe095b039bb559fd7fe2e36585f784ad8d85819fd6706754324741749d0` | the same tests, on this harness's tool interface |

<!-- carried-tools:end -->

**bytes**, **SHA-256**, **line share** and **longest run** are of the staged
original, as in the table of files; the scan's **verdict** is CARRY for a
file that used nothing of the earlier harness and ADAPT for one that did,
and the count beside it is how many of that harness's symbols the file used
(the scan's `n_crush_syms`; `adapter-surface.tsv` names them).

### What was kept, and what was replaced

How much of each staged file stands in the file made from it, counting its
lines that are not blank and that are here in the same order, indentation
aside: `tools.go` 486 of 662, `memtodos.go` 49 of 66, `tools_test.go` 81 of
103, `closecheck_test.go` 129 of 137, `memtodos_test.go` 37 of 57,
`namer_test.go` 22 of 26, `docs_test.go` 708 of 791.

The counts for `memtodos.go` and `memtodos_test.go` were taken before the
task-list type was written anew (see `memtodos.go` below). That change
touched, besides the declaration itself, the ten lines of `memtodos.go` and
the fifteen of `memtodos_test.go` that name the type, its states or its
fields. The staging folder was not at hand to count again: any of those
lines that stood as it was staged no longer does, so the two counts may be
lower by as many.

**`tools.go`.** Kept, with their texts and their logic as they were: the
four descriptions; the four argument structs with their tags; the rendering
of a run's result (the value and output, the two error prefixes, the `Next:`
hints, the status line, the notes, the cut at 30,000); the rendering of
`gnovm_new`'s answer and of `gnovm_list`'s; the finding of the texts a call
keeps in mem without naming them; the gas argument's conversion; the
close-check loop; what is said of a call whose worker died before it
replied; and the order in which a kept call's frames, goals, erasure and
rules are handed on, with a context that cancellation does not reach.
Replaced:

| the staged file's call into the earlier harness | here |
|---|---|
| the tool constructor of the earlier harness's tool library, called with a name, a description and a function over a typed argument struct whose tags name and describe the arguments | four types that implement `core.Tool`; `params.go` makes the JSON schema from the same structs and tags, and reads a call's arguments into them |
| that library's text, error and with-metadata responses | `core.TextResult`, `core.ErrorResult`, and `ToolResult.Meta` under the key `frames` |
| the calling session's id, read out of the context | `core.ToolEnv.SessionID`, or the `Session` function of `Options` |
| the `*gnovm.Lazy` and the project given to each constructor | `Options.Client`, an interface that `*gnovm.Client` satisfies, and `Options.Project` |
| the functional options of `gnovm_run` (`MemStack`, `BoundGoal`, `MemFresh`, `Namer`, `CheckCloses`, `ClosesKept`, `SubAgent`, `OnMemFrames`, `OnMemGoals`, `OnMemErased`, `OnMemRules`) | fields of `Options` with the same names and the same function types; `SubAgent` is a function now, since one set of tools may serve several sessions |
| the permission service asked, inside `gnovm_drop`, before a delete, and its answer "the user did not allow deleting VM x" | `Describe`, which every tool has now: it says in one line what a call would do, and the harness's permission rules decide and ask. The line for a delete is the one the person was asked with. The refusal's old words are kept as `DeleteRefused`, which nothing here calls |

Added, with no counterpart in the staged file: `Describe` for all four
tools; what a `gnovm_run` call shows a person (`core.Display`); `FramesOf`;
and checks of what the harness hands the tool (a way to the GnoVM, a
`ToolEnv`, a project, a session id, an answer from the client), whose
failure is the harness's and not a result for the model. What a call is
told when its arguments are not of the types the schema gives (`gas must be
a number`) is worded here: the staged file left that to the tool library.
`Describe` fails only for a delete that could not run, so that nobody is
asked about one; every other call whose arguments cannot be used is told so
by `Run`, in the staged words, and not in the sentence the loop wraps a
`Describe` error in. One thing was changed on purpose: the cut at 30,000 is
made between characters and never inside one, where the staged code cut at
the byte.

**`memtodos.go`.** It turned a goal of mem's into the earlier harness's
session task list. Kept: which line is in progress, which are pending or
completed, and which are left out. Replaced: the earlier harness's todo type
and its three statuses, by a task-list type that is gnosh's own, declared in
the file itself; and the setter, which returned the earlier harness's session
and an error that this file logged. The setter returns nothing now, and the
log line is gone: a list that cannot be written is the setter's to tell the
person of.

The task-list type is `TaskLine`: a line's `Title` (the todo's words, as mem
holds them), `AtWork` (how the line reads while it is the one being worked)
and `State`. `LineState` is a small integer, `LineOpen`, `LineWorking` or
`LineDone`, with a `String` method that names it in a word. The type was
written twice. As the file was first ported it declared a string type with
three string constants and a three-field struct, under the names the staged
file's call sites had used for the earlier harness's type; the similarity
audit (`tools/audit/similarity/README.md`, finding 1) found those eleven
lines to be, token for token but for one struct tag, a declaration of the
earlier harness's Crush-derived code. They were taken out, and the type was
written anew after the audit, without looking at Crush or at the earlier
tree: none of the old type's, constants' or fields' names is kept, and no
alias stands in for them. What `MemTodos` decides is as it was, and every
string the staged file held is still here: the three status words and the
three struct tags of the first declaration were the porter's, never among
the staged strings the check holds, and went with it. `internal/memjoin`
fills the front end's task from a `TaskLine` (the front end's published
names are its own, in `internal/app`), and keeps a session's task list in
the session's file in words of its own.

**`tools_test.go`, `closecheck_test.go`, `memtodos_test.go`,
`namer_test.go`** are the same tests. The tools are made with `New` and
called through `core.Tool`; the session id is given in the `ToolEnv`; the
assertions are the same lines, checked by `check_test.go` on the standard
library instead of by the testify module. `tools_test.go` no longer makes a
permission service for the delete, and `memtodos_test.go` no longer runs in
parallel with the others.

**`docs_test.go`** replays the examples of `GNOMEM.md` through the real
tool, as it did, and checks its links, its quoted prompts and its table of
runs. Left out, because what they read is not in this repository: the same
checks of the earlier gnosh's `README.md` and of three documents under
`docs/`; the test that ran the README's install lines; and
`TestDocUserWordVerdict`, which tied the words GNOMEM quotes before the
reason of a goal met on the user's word to a constant of the earlier
harness's goal driver. That tie has no home until this harness has a goal
driver. `TestDocLoopNote` held the README's quotation of the scratch
machine's note to the note; it now holds the note to its words as
`spec/gnosh/machines.md` records them.

### What the check can and cannot hold these files to

`tools/audit/carried.py check` cannot undo these changes to get the staged
originals back, as it does for the table of files. For this directory it
checks instead that:

- the directory holds exactly the seven files above and the four listed
  under "Files written fresh", and each of the seven staged files is the
  source of one;
- the table above is the one the script writes from its own record of the
  staged originals, so that a size, a checksum or a number cannot be changed
  in one place alone;
- every string the staged `tools.go` and `memtodos.go` held is still, byte
  for byte, a string of the files here that are not tests. That is 109
  strings: the descriptions, the argument structs' tags, every format and
  every fixed text of a result or an error. Three more are gone, and are
  listed in the script with the reason: `memtodos.go`'s log line and the
  key of its attribute, and the tag of the one-field struct that held a
  result's frames, which are kept under the same key, `frames`, in the
  result's `Meta`. The strings are in
  `tools/audit/gnovm-tools-strings.json`.

`tools/audit/carried.py tools --staged DIR --scan FILE`, run with the
staging folder at hand, checks the script's record against the staged files
themselves (size, SHA-256, the scan's verdict and numbers) and that every
string of the staged `tools.go` and `memtodos.go` is either kept or listed
as dropped, and writes the table and the list of strings.

What no script checks: that the code around those strings does what the
staged code did. The package's tests are for that. They run the tools
against the real host, in the process, and against a scripted client, and
hold each result to the text `spec/gnosh/machines.md` gives.

## Words carried into `internal/prompt`

The table above is of files. Some of what gnosh says to a model was carried
too, as words inside files that were otherwise written here, and an audit
(`tools/audit/similarity/`, finding 2) found that this file did not say so.
It does now. None of these words is Charm's: the inventory checked every
line of them absent from Crush's templates at the fork point and from the
rest of Charm's code, and the audit, measuring by sentences and by runs of
words, found none of the 331 sentences of `internal/prompt/texts` in any
version of Crush.

| text in `internal/prompt/texts/` | where the words come from in the earlier gnosh | how much |
|---|---|---|
| `close-review.md` | `internal/agent/templates/close_check.md`, a file the earlier gnosh added (staged) | 22 of 23 sentences |
| `goal-judge.md` | `internal/agent/templates/goal_check.md`, added (staged) | 8 of 11 sentences |
| `memory.md`, `memory-full.md` | the block about mem that the earlier gnosh wrote into its coder prompt template. That template was a Crush file the earlier gnosh had modified; the block was the earlier gnosh's own addition, cut out by the inventory as `own-text/coder-prompt-memory-block.md` | most of both texts |
| `gno.md` | the block about Gno projects in the same template (`own-text/coder-prompt-gno-block.md`) | nearly all |
| `deliver.md` | the block about deliverables in the same template (`own-text/coder-prompt-artifacts-block.md`) | about three quarters |
| `mem-line.md`, `frame-line.md`, `close-entered-here.md`, `close-sendback.md` and about ten more short texts (receipts, refusals, send-backs) | messages in Go files the earlier gnosh added (`mem_line.go`, `close_check.go`, `refused_close.go`, `frames.go`, `mem_reminder.go`) and, for two, its hooks documentation; most reached the build side quoted in `spec/gnosh/` | a sentence or two each |

These words were tuned against a real model over weeks of runs, which is why
they were kept. Everything else in `internal/prompt/texts` was written for
this repository.

## Files written fresh

These were written for this repository, from what the carried code needs of
them and from public documentation. None was copied or adapted from the
earlier tree. `internal/filelock` and `internal/buildinfo` stand in for two
helpers of the earlier harness that were not staged and that the porter did
not read. `sqlite.go` and the two `racedetector` files stand in for the four
files of `internal/gnovm` that the inventory did not stage because they
resembled Crush's: two SQLite openers, one for each of two drivers, and two
race-detector tag files.

| file | why it exists |
|---|---|
| `internal/filelock/filelock.go`, `filelock_unix.go`, `filelock_windows.go` | Advisory file locks: `TryFile(path)`, `File(ctx, path)` and `ErrContended`, which is all the carried code used of the earlier harness's lock helper. What the callers and their comments rely on is written at the top of `filelock.go`: an exclusive `flock(2)` lock (the carried code probes and waits on the same files with shared `flock`s of its own), one holder per call even inside one process, a wait that is the kernel's and ends early only with the context, a lock file that is made on first use and left in place. The Windows file (`LockFileEx` on one byte) has been compiled and vetted for Windows, and never run there. |
| `internal/filelock/filelock_test.go`, `filelock_unix_test.go` | Its tests, including the lock seen from another process and granted when that process dies (through `flock(1)`, where it is installed). |
| `internal/buildinfo/buildinfo.go`, `buildinfo_test.go` | `BuildID`, the one name the carried code used of the earlier harness's version package: set at link time with `-X`, or else taken from the build information the go command records (commit, `+dirty`; else module version; else `devel`). A gnosh process and the worker it starts refuse each other when theirs differ. |
| `internal/gnovm/sqlite.go` | `openSQLite(path, pragmas)`, the one opener for mem's index and vectors, on `modernc.org/sqlite` (SQLite in pure Go; there is no second driver and no build tag). Its comment gives the reason for each setting, from SQLite's documentation. |
| `internal/gnovm/sqlite_test.go` | Tests of the opener: the path it opens, pragmas on every connection, transactions that take the write lock at `BEGIN`. |
| `internal/gnovm/racedetector_on_test.go`, `internal/gnovm/racedetector_off_test.go` | `raceEnabled`, true or false by the `race` build tag, which two carried tests read to drop their timing checks under the race detector. |
| `internal/gnovm/tools/options.go` | `Options`, everything the four tools are given from outside, each field with what it must return and when it is called; `Client`, the interface the tools reach the GnoVM through, and `Lazy`, which makes one of the program's `*gnovm.Lazy`; `New`, which makes the four. It takes the place of the earlier constructors' arguments and functional options. |
| `internal/gnovm/tools/params.go` | The JSON schema of a tool's arguments, read off the argument struct and its tags, and the reading of a call's arguments into the struct. The earlier harness's tool library did both; this is written from what the structs' tags say and from JSON Schema's and `encoding/json`'s documentation. |
| `internal/gnovm/tools/check_test.go` | The checks the tools' tests make (`require` and `assert`, on the standard library alone, with the few methods those tests use), and the helpers that call a tool as a session. |
| `internal/gnovm/tools/run_test.go` | Tests written for this harness: every result form of `machines.md` section 9 against the real host, the request of `integration-contract.md` section 5.2, the order in which a kept call's frames, goals, erasure and rules are handed on, the close-check loop and a worker's death against a scripted client, `Describe`, what a call shows, the cut at 30,000, cancellation, and the parsers under fuzzing. |
| `tools/audit/carried.py` | Writes and checks the tables in this file. |
| `tools/audit/gnovm-tools-strings.json` | The strings of the staged `tools.go` and `memtodos.go` that the files here keep, written by `carried.py tools` and read by `carried.py check`. |

## Licences and notices

Each carried subtree has its licence and notice files beside it, unchanged
(their checksums are in the table):

- `internal/gnovm/LICENSE.md`: the GnoVM subsystem's own notice, the GNO
  Network GPL v6 and its Additional Terms. It covers every file under
  `internal/gnovm/` but the two subtrees below, so it covers the files
  written fresh there too, which carry the same header as their neighbours.
- `internal/gnovm/stdlibs/LICENSE.md`, `LICENSE-GO` and `VERSION`: gno's
  licence under a short preface, Go's BSD licence for the files that came
  from Go, and the gno version copied.
- `internal/gnovm/klashlibs/klash/bptree/v0/LICENSE.md` and `SOURCE`: gno's
  licence for the three files of its B+tree, and where they came from.

The porter checked these against gno's module in the Go module cache: all
160 `.gno` files of `stdlibs/` and the three of the B+tree are byte for byte
gno's at v1.5.1-0.20260928125126-5cdbc25fcde0, both `LICENSE.md` files are
gno's `LICENSE.md` after their prefaces, and `LICENSE-GO` is Go's `LICENSE`.

`gno/r/gnosh/mem/`, `internal/gnochain/` and `internal/swarm/` came without
a licence file of their own, as they were.

## Tests

The porter removed, changed and skipped no test. The suites ran on the
devbox (Ubuntu 24.04, 16 virtual CPUs of a Xeon Platinum 8168, go1.27.1
linux/amd64) on 13 November 2026.

| what ran | result |
|---|---|
| `go build ./...` and `go vet ./...` | pass; also cross-compiled and vetted for darwin/arm64, windows/amd64 and linux/arm64 |
| `go test -count=1 ./internal/filelock/ ./internal/buildinfo/ ./internal/gnochain/ ./internal/swarm/...`, and the same with `-race` | ok (`swarmdemo` has no tests) |
| `go test -count=1 -v ./internal/gnovm/` | ok in 269 s: of 218 tests, 210 pass and 8 skip themselves |
| `go test -race -count=1 -v ./internal/gnovm/` | ok in 1,256 s: the same 210 pass and 8 skip, and no data race is reported |
| `gno test -v .` in `gno/r/gnosh/mem`, with the gno tool built at the pinned version and `GNOROOT` pointing at gno's module | ok: 24 tests pass. Nothing in `go test` runs these; they need that tool. |

### The tools' tests

`internal/gnovm/tools` was brought later the same day. Of the 20 tests in
its staged files, 18 were brought. Two were not: the one that ran the
earlier README's install lines, and the one that tied GNOMEM to a constant
of the earlier harness's goal driver. Five of the 18 read `GNOMEM.md` only,
where they read the earlier README too ("The tools, carried with larger
changes" above). 25 tests were written for this harness, two of them fuzz
tests of the parsers.

| what ran | result |
|---|---|
| `go vet ./internal/gnovm/tools/`, and the same for darwin/arm64 and windows/amd64 | pass |
| `tools/dev/remote-test.sh -race -timeout 60m internal/gnovm/tools internal/gnovm internal/filelock internal/buildinfo` | ok, with no data race reported: the tools' 43 tests in 207 s, `internal/gnovm` in 1,675 s, `internal/filelock` and `internal/buildinfo` in 1 s each. Other builders' tests and fuzzers kept the machine's load average between 7 and 46 meanwhile, and the timing checks below held all the same; an earlier run of the same command, half an hour before and on the files as they then stood, also passed |
| `go test -run '^$' -fuzz` of `FuzzDecodeParams` and of `FuzzUnnamedTexts`, 40 s each | pass, after 495,282 and 263,983 inputs |

The tools' tests start no worker: the client runs its host in the test
process (`ClientOptions.InProcess`), or is a scripted one. They read
`GNOMEM.md` at the repository root, as `internal/gnovm`'s
`TestDocMemKeepsWhatAnEarlierBuildKept` does. `tools/dev/remote-test.sh`
copies package directories and not that file, so it was copied to the
devbox by hand for these runs. Without `-timeout` the script's `go test`
stops a package after ten minutes, which `internal/gnovm` outlasts under
the race detector.

### The timing checks need a quiet machine

Some of the carried tests measure time. `TestPeersDemonstration` wants three
peer processes to finish more than 1.6 times as fast as one; the tests of
waiting sessions give a wait 20 s to begin. On the devbox they pass when
nothing else runs there, and can fail when something does. The results above
are from a quiet machine. Earlier in the day, with other builders' tests and
fuzzers keeping every CPU busy:

- without the race detector, `TestPeersDemonstration` missed that one bar in
  seven runs (three peers were 1.36 to 1.57 times as fast as one, a peer's
  start taking 2.7 s, at normal and at raised priority alike) and every
  other test passed; quiet, the same test measured a start of 1.6 s, and
  1.69, 1.71 and 1.75 times for three peers, for eight, and for three with
  a planner;
- with the race detector, `TestClientNextWaitsAndWakes`,
  `TestPeersQuietHolder` and `TestPeersKilledHolder` ran out those 20 s, and
  `TestDeclarationChargeCoversHeap` then measured the heap of the work they
  had left running. A test that fails this way also sets off a data race
  report, between its own clean-up and the goroutine it left behind; it is
  the failed test's, not the code's. Run one at a time at high priority,
  all four passed even then.

None of it is the port's doing: a CPU profile of a first mem call and forty
more puts 2.4 of its 5.6 s in gno's compile of mem's libraries and 0.07 s in
the SQLite opener, and the file lock does not show in it.

### Tests that skip themselves

Each of these was written to run only when it is given something from
outside, and says so when it skips:

| test | what it needs |
|---|---|
| `TestScenarios` (`internal/gnovm`) | `GNOVM_SCENARIO_GNOSH`: a gnosh binary, a real model behind it, and the earlier harness's configuration files to copy. It cannot run until this harness has a binary and its own configuration; its set-up then needs pointing at that. |
| `TestRealWorker` (`internal/gnovm`) | `GNOVM_WORKER_BIN`: a gnosh binary built from this tree, which this repository does not have yet (its `gnovm-worker` argument must call `gnovm.WorkerMain`). See "The worker, by hand" below. |
| `TestMemRecallLive` (`internal/gnovm`) | `GNOSH_TEST_EMBEDDINGS`: an embedding server. |
| `TestMemHostErasesAtScale`, `TestMemRealmScale`, `TestMemRecallScale` (`internal/gnovm`) | `GNOMEM_SCALE`: opt-in measurements at a size. |
| `TestPeersSwarmControl` (`internal/gnovm`) | `GNOSH_PEERS_SWARM` and `GNOSH_PEER_WORK`: an opt-in measurement. |
| `TestPeerChild` (`internal/gnovm`) | It is the body of a peer process that the peers tests start; on its own it does nothing. |
| `TestLive` (`internal/gnochain`) | `GNOCHAIN_LIVE_RPC` and a key: a live chain. |

### The worker, by hand

`TestRealWorker` runs the VMs in a real worker process, and needs a gnosh
binary for it; this repository has no `main` yet. So that the worker's path
was run at least once, the porter ran the test on the devbox against a
stand-in kept out of the tree: a `main` whose only act is to call
`gnovm.WorkerMain` when it is started as `gnovm-worker`. With the same build
id linked into the test binary and the stand-in, as the test's comment
tells (`-X github.com/gnosh-ai/gnosh/internal/buildinfo.BuildID=gnovm-it`),
the test passed. With no id linked into either, it passed. With different
ids the worker was refused in the handshake, as it must be: "worker is build
another-build (gno v1.5.1-…), client is gnovm-it (gno v1.5.1-…): restart
gnosh".
