# Historical revision source

This directory preserves reconstructable full-index deltas for tournament-worthy eventloop revisions that are not reachable from a durable Git ref. Each patch is applied to its pinned reachable parent, and retention tests require the exact resulting root tree.

| Source record | Parent | Root tree | Eventloop tree | Patch SHA-256 |
|---|---|---|---|---|
| `60f89427c8b36ecb2b1c495309ac91187c26fd06` | `cd6a53322588420c9e2b5e19e5791b7b0696117f` | `7d99eec56bf840cb4b832c29d1072b355a707910` | `57b88dedc97ec680e4915cbf7b0181f3def008b7` | `5a5d995a3988240248f8b6f2b60ad623b23a4ba66c209ed87d0f28be264bf7dd` |
| `819ee003aa8ff73d51e5a43dd35169db784f3111` | `cd6a53322588420c9e2b5e19e5791b7b0696117f` | `93f2b311557b83249ca59b5f9646f9a2677a3944` | `35d7e1cfa4dc7d7485a13b39d7c55ad7fcffae83` | `434858ef0d7c4940859f0b618c1ba62d0d85e5f56af1d0dcdec4f9bfc43cd0ee` |
| `81caf61f96167e7b7f5ecf497af9110890ff6e03` | `cd6a53322588420c9e2b5e19e5791b7b0696117f` | `7d387838addad98726792c47e30d4e3b7f824e88` | `115d23747d2fe938c88b440e822c9bb40e2f61fe` | `d8839deb295d0c82ab7a0b8f9f87eb71308efcd6006b5637fe2f1e1164f008f9` |
| `3249ec949f50e3743b34cdaf831728ec86e8796b` | `9b77ad1d20f093759da7e0ff4a85fa50b5cf6f15` | `7328ba8bdf18a3262a61761a2c13265e791659f4` | `3d57c2de57c10cac02ea7f05f9a83c10ca9846f1` | `34f0f29d9ac9f581b5e076bb45fb43bb8b197d8a6f3e1f29f440915e6580e865` |
| `f7ef4c86843e1790bf0528975ab2c92ba3351702` | `c8e744e4867c351d5b83e438fd2cb438c9b04898` | `c28e4b2a8cc0acf2f7795f3c82666273ec5dd6ec` | `6eec43e57bfc858888a804e206b0d97335ba6d89` | `1ae62529924d910ba7f038a8afdd7d5bbf54c2876100b033ccc3e152ab80f48a` |

Every record above also has goja-eventloop tree `69d8cf81666942396704d3d4bdb75208a0e523c6`.

## Unreachable commit objects

Source patches preserve trees, not commit object bytes. The `commits/` directory therefore stores a lossless single-line base64 encoding of every unreachable commit payload. Base64 is necessary because two original payloads do not end in a newline. The retention gate decodes each payload, checks its byte count and SHA-256, then hashes the exact Git object framing `commit <length>\0<payload>` and requires the filename's SHA-1 object ID.

| Commit object | Payload bytes | Payload SHA-256 |
|---|---:|---|
| `3249ec949f50e3743b34cdaf831728ec86e8796b` | 283 | `43f0ba8550c35e6b1a47200e66c3247622bf0d24a5f06067877362eddd826dc4` |
| `537692fbb199d9e7e13255260ae878faff94fc02` | 320 | `1cd4d35604cae3f01df021aa7136d7b0a7a7080e4ca2e39045c55e658fd8cd70` |
| `5f691abe5d4557c13eb7c16d42f60f203df24336` | 261 | `ab1c1b4beaf797cbcb78e134a30a408b48b3a903cce3528d8221b504830df9f7` |
| `60f89427c8b36ecb2b1c495309ac91187c26fd06` | 261 | `597ff77baa81e2109bf1475b06b4f75a4950551fc322438400462b038eedbfcd` |
| `786998e8eb654d670dcf2174c1e362c784a08d3e` | 403 | `2b485f621d7ab4392b7edf904184e1f0a98d6e1e73d5469a6206bd5e70e9bcfb` |
| `819ee003aa8ff73d51e5a43dd35169db784f3111` | 261 | `e4bb6d29fac32e6f5c573b928b37c4bf909a61f01a307eb16ed92b96613eb25f` |
| `81caf61f96167e7b7f5ecf497af9110890ff6e03` | 261 | `b1c9083de73e77ff2f0c2825ec7753a77fa856dcc417fd5a06a82097ac91f6a9` |
| `b3c342c981169d1b8b348c81a8e850a8a87911ee` | 342 | `48e18fca44ecc884e675d642cbd7800e105d964c19888cd30d4c44d55b328a5b` |
| `bcadd4aaa61c7a9c1068a493c948bb88bf5fa038` | 297 | `5def94f6c0874461c531a6e2e71d490d9a33e57579795676a02b44b697951206` |
| `f7ef4c86843e1790bf0528975ab2c92ba3351702` | 403 | `7ec816b4bb6c607ac2dc2105e65ff05800d3a3f593746d2ba657b2a47b8f4c83` |
| `f97fc084bb6a796dac41461a01f059a5845fbe47` | 226 | `5f54b38bfc32fd492f8231be1d625f7648c2954b1906a1e04ef26229b97e194c` |
| `53e2f662adc245c9b63e06bb64977b0751dcff82` | 305 | `5757cc94cc79fcc877e8ff8d0b8626f6835fb5efa29cbc6ca0cf188420424c65` |
| `1396868d29689c659ff7782760e89423aa478cf4` | 350 | `f85df86a07aa8d4effc78cc3126ec493c2d5d9db5dc0da34fcd73628e4d04821` |

History generation first seeds a sanitized temporary bare SHA-1 repository with only the fixed ancestry ending at `469fd952`, then requires all thirteen archived commit IDs to be absent. It reconstructs the five missing trees through isolated indexes, writes the thirteen verified raw commit payloads as Git objects, runs strict object verification, and generates the registry only from that rehydrated authority. The retention owner independently applies each patch through a private index and object directory with reachable objects as read-only alternates. Neither path checks files out, so host file modes, filters, line-ending conversion, symlink support, ignored files, and umask cannot alter the proof.

The following unreachable objects are provenance aliases, not additional executable source variants:

- `537692fbb199d9e7e13255260ae878faff94fc02` has the same root as `3249ec949f50e3743b34cdaf831728ec86e8796b`.
- `786998e8eb654d670dcf2174c1e362c784a08d3e` has the same root as `f7ef4c86843e1790bf0528975ab2c92ba3351702`.
- `5f691abe5d4557c13eb7c16d42f60f203df24336` has the same root as reachable `fa68be11`.
- `bcadd4aaa61c7a9c1068a493c948bb88bf5fa038` and `b3c342c981169d1b8b348c81a8e850a8a87911ee` have the same root as reachable `8bbefe5623c5b94cd85aa8dda2f3ebe9007d3eba`.
- `f97fc084bb6a796dac41461a01f059a5845fbe47` has the same root as reachable `0def02e2`.
- `53e2f662adc245c9b63e06bb64977b0751dcff82` and `1396868d29689c659ff7782760e89423aa478cf4` have the same root as reachable `0bc4ad0a`.

WAKE-H2 records `819ee003` and `81caf61f` have the same Darwin/Linux production bytes but distinct full cross-platform trees; the latter changes Windows production. They remain separate source records while platform-specific analysis may record their proven runtime equivalence.
