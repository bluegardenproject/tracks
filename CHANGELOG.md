# Changelog

## [1.1.1](https://github.com/bluegardenproject/tracks/compare/v1.1.0...v1.1.1) (2026-09-07)


### Bug Fixes

* **daemon:** only adopt GitHub URLs from the PR marker ([37bc7f1](https://github.com/bluegardenproject/tracks/commit/37bc7f12f01312013ca0494acef12d35bc574f69))
* **dashboard:** strip control characters from externally-supplied text ([d2e5b20](https://github.com/bluegardenproject/tracks/commit/d2e5b201dcbd211945f717fb633d7f25997089b9))
* **notify:** escape backslashes before quotes in AppleScript strings ([36d5980](https://github.com/bluegardenproject/tracks/commit/36d59801d63dbf330a270bf3e2e7db6408bf01fe))
* **update:** finish the de-branding pass and the verify story ([edd40b9](https://github.com/bluegardenproject/tracks/commit/edd40b92bf3fe31115e295526a9b918991588208))
* **update:** verify a release download before running it ([77668c1](https://github.com/bluegardenproject/tracks/commit/77668c1ba115bd21c6198e7a2f4aad3135b1018a))


### Miscellaneous

* drop project-specific references from docs and fixtures ([53b4e13](https://github.com/bluegardenproject/tracks/commit/53b4e1353a5933ed1b6069006f0029191cb100e5))

## [1.1.0](https://github.com/bluegardenproject/tracks/compare/v1.0.1...v1.1.0) (2026-09-02)


### Features

* **cursor:** don't cost a Cursor track against Anthropic rates ([3a69492](https://github.com/bluegardenproject/tracks/commit/3a6949228722c7c58395f67128159a1f3325cfb9))
* **cursor:** install the tracks rule into ~/.cursor/rules ([ef1a6ce](https://github.com/bluegardenproject/tracks/commit/ef1a6cee9a9f41d1942b4541309982c30e0b601e))
* **cursor:** launch Cursor Agent as a track's provider ([ed272f6](https://github.com/bluegardenproject/tracks/commit/ed272f60f89efd0d3551a8faee91367b673b24fe))
* **cursor:** review with a second agent, not with itself ([f1f1bea](https://github.com/bluegardenproject/tracks/commit/f1f1bead04ea5887a8dc4f078ba9726206628572))
* **tracks:** carry a provider on every track ([b2ccf2c](https://github.com/bluegardenproject/tracks/commit/b2ccf2ca85b431f8e4eda56fbca3cc26e558cb70))
* **tracks:** launch the provider the track was created with ([f3a6ecf](https://github.com/bluegardenproject/tracks/commit/f3a6ecfe66fdbe87ba3ea67c51028b73f2ca0327))
* **tracks:** pick a provider when creating a track ([b2e3955](https://github.com/bluegardenproject/tracks/commit/b2e395562292e756f6c92f57499220fb1bdee750))
* **tracks:** reopen every track you had open, not just the running ones ([4940eaa](https://github.com/bluegardenproject/tracks/commit/4940eaae77b47281ed7bf495de1fc27c289e7e6f))


### Bug Fixes

* **cursor:** rune-safe truncation, and drop a constant nothing reads ([936b1a3](https://github.com/bluegardenproject/tracks/commit/936b1a369638263da343d8844760e40ef01c92a9))
* **daemon:** never overwrite a home-directory file tracks doesn't own ([9ab3d20](https://github.com/bluegardenproject/tracks/commit/9ab3d209c6572e9532cea102bfa888e7496eda33))
* **tracks:** diff the changed-file list from the merge-base ([fbbf16f](https://github.com/bluegardenproject/tracks/commit/fbbf16fbad9e3922713709e8922722e480084a67))
* **tracks:** keep a PR-open track reachable across a restart ([8221e4e](https://github.com/bluegardenproject/tracks/commit/8221e4e3cb7d077be2fe08572d9672fb404b0389))
* **tracks:** say when the detail panel couldn't read a repo ([62c06bf](https://github.com/bluegardenproject/tracks/commit/62c06bf5025ef7b4da6b1b6b15fb62a5d543227b))


### Performance Improvements

* **tracks:** rate-limit the git polling behind the dashboard ([94259fd](https://github.com/bluegardenproject/tracks/commit/94259fdb6c0f44a09f82a7f3d055aa210d28bbbd))


### Code Refactoring

* **agent:** share the pane scaffolding between providers ([6a3bdcd](https://github.com/bluegardenproject/tracks/commit/6a3bdcdd2ea11a6ae4e2e129715e2417a2ccd3eb))
* **shellx:** one shell-quoting implementation, two named behaviours ([d9867cb](https://github.com/bluegardenproject/tracks/commit/d9867cbd660ad3a666f36124089da76c1a0f025b))
* **shellx:** quote globs, and stop the test passing by accident ([73ed28f](https://github.com/bluegardenproject/tracks/commit/73ed28fce09fb6fe4c9bdf2942a31056e134af3b))
* **tracks:** clean up the diffstat leftovers ([621e196](https://github.com/bluegardenproject/tracks/commit/621e196ef106f8b9affa315c78f9d62c3d2c73ee))
* **tracks:** drop the per-tick diffstat ([d770f13](https://github.com/bluegardenproject/tracks/commit/d770f138e807973cd837aa4770a429e074bfe056))


### Documentation

* **roadmap:** the shellQuote half of "shared helpers" is done ([5f54d1e](https://github.com/bluegardenproject/tracks/commit/5f54d1e672b604085cc2e7fe7af27b0cb799fb9b))

## [1.0.1](https://github.com/bluegardenproject/tracks/compare/v1.0.0...v1.0.1) (2026-08-27)


### Documentation

* **design:** Cursor CLI integration research and design ([89a1e9d](https://github.com/bluegardenproject/tracks/commit/89a1e9de5eb2d7771d2dff977c073c500d54af80))

## [1.0.0](https://github.com/bluegardenproject/tracks/compare/v0.7.0...v1.0.0) (2026-08-27)


### ⚠ BREAKING CHANGES

* **state:** state.json is written as schema_version 5. An older tracks binary refuses to load it ("schema_version 5 newer than supported") and will not start against a state file this version has written. Downgrading means restoring a backup of state.json.

### Features

* **dashboard:** drop the CHANGES column from the track table ([a90c21c](https://github.com/bluegardenproject/tracks/commit/a90c21c2583ca084ce6f8b05077da08a5665265c))
* **dashboard:** report a track's sub-agent model alongside its own ([d25e43c](https://github.com/bluegardenproject/tracks/commit/d25e43c1b1278b2a35fec08d63b0eaf25ee979ea))
* **dashboard:** show each track's model, and size the table to the terminal ([2919eb2](https://github.com/bluegardenproject/tracks/commit/2919eb296508df15634c893fe4b51f0f3000e9a3))
* **settings:** add a Models section ([8ec3ba0](https://github.com/bluegardenproject/tracks/commit/8ec3ba0c926f96cc7d89d542b8a174114190ccd9))
* **tracks:** choose a model when creating a track ([522ee3c](https://github.com/bluegardenproject/tracks/commit/522ee3c68ab330e04c745621e9b20fbea0397646))
* **tracks:** name tmux tabs after the track, not the track id ([115f6a1](https://github.com/bluegardenproject/tracks/commit/115f6a1f989ab9165a3625c5fe9e8d832ebf1252))


### Bug Fixes

* **usage:** price each model version, not each family ([c4207ad](https://github.com/bluegardenproject/tracks/commit/c4207ad3d9014c4df3716ad927837e63c563fe2f))


### Code Refactoring

* **state:** move review/doc fields into sub-structs, drop LogPath ([f577705](https://github.com/bluegardenproject/tracks/commit/f577705196b8b93f27f942fdc2d864e4fdbb602e))
* **state:** name the derived model ObservedModel ([270d079](https://github.com/bluegardenproject/tracks/commit/270d07900dd9521fab062dd066ffe7820ad35938))


### Documentation

* **roadmap:** record the open 1.0 review backlog ([bc9a2af](https://github.com/bluegardenproject/tracks/commit/bc9a2af2e448c8f25d80c244e733e00dca2850d2))
* **roadmap:** record the proxy review findings ([6fa324d](https://github.com/bluegardenproject/tracks/commit/6fa324dedffe7bf680b5d1b4a7dd15abe99738ba))

## [0.7.0](https://github.com/bluegardenproject/tracks/compare/v0.6.0...v0.7.0) (2026-08-18)


### Features

* **proxy:** user-defined stable ports, decoupled from service config ([c5a8fdb](https://github.com/bluegardenproject/tracks/commit/c5a8fdb5d848622cf70a5296d24384eeb9cc1d55))
* **review:** sharpen the doc reviewer's opinions and findings ([f27d0d8](https://github.com/bluegardenproject/tracks/commit/f27d0d87cb046b774d88b080b0ea9edcac3428b9))

## [0.6.0](https://github.com/bluegardenproject/tracks/compare/v0.5.0...v0.6.0) (2026-08-14)


### Features

* **daemon:** write the daemon log to a file ([5be2459](https://github.com/bluegardenproject/tracks/commit/5be2459b097d2e84e573d08aed10a44c98b0b224))
* **menu:** check for updates and self-install a newer release ([a2c246d](https://github.com/bluegardenproject/tracks/commit/a2c246d495d710c3cc67877aa2bcee3e28930fa0))
* **services:** make the ready probe and post_start hooks real ([64fe38b](https://github.com/bluegardenproject/tracks/commit/64fe38b83db9f41727e2f6091e0778086fad91ad))


### Bug Fixes

* **daemon:** guard the supervisor's observation state with its own lock ([5032f81](https://github.com/bluegardenproject/tracks/commit/5032f815a6483fb1fb820e0bcdf4a5e5daaaf963))
* **daemon:** log the state writes that used to fail silently ([21602c2](https://github.com/bluegardenproject/tracks/commit/21602c2e092b6f57e97e7b305d179f2b07a863e0))
* **proxy:** bind stable ports to loopback, and follow config reloads ([a842810](https://github.com/bluegardenproject/tracks/commit/a842810974bd99125a86ca84c278acc7839f727f))
* remove the permission-prompt path that nothing fed ([160c5f0](https://github.com/bluegardenproject/tracks/commit/160c5f06c89ff21b789c57eab4226fd97b08f815))
* **update:** count only the tracks the restart actually interrupts ([faf9081](https://github.com/bluegardenproject/tracks/commit/faf9081364c2d5d2b9cdb3cf534382fd1683c514))
* **update:** state the real cost of the next tracks run ([e521a50](https://github.com/bluegardenproject/tracks/commit/e521a50eff0e55f4bae3a447cc9b0a174b8ae0de))


### Code Refactoring

* drop code left dead by earlier rewrites ([bf25d2f](https://github.com/bluegardenproject/tracks/commit/bf25d2fad6f3bfaf49fced21a6a518c05262a974))


### Miscellaneous

* add MIT LICENSE ([cace206](https://github.com/bluegardenproject/tracks/commit/cace206076344fedec9a1d79e4f97d1ded15a4ff))

## [0.5.0](https://github.com/bluegardenproject/tracks/compare/v0.4.1...v0.5.0) (2026-08-07)


### Features

* **review:** candor dial, optional claim check, opinion section ([0d29f66](https://github.com/bluegardenproject/tracks/commit/0d29f6650a84838114bd40d5500c333dbd0ae0f2))
* **state:** per-PR track state with pr open / pr merged statuses ([7f3f332](https://github.com/bluegardenproject/tracks/commit/7f3f332df6a3410e7b5b3b75406224d9d8d13f11))


### Bug Fixes

* **services:** stop the runner tests racing the shell and the kernel ([4156bfe](https://github.com/bluegardenproject/tracks/commit/4156bfe85e6b18194c853420e087b79489918290))

## [0.4.1](https://github.com/bluegardenproject/tracks/compare/v0.4.0...v0.4.1) (2026-08-06)


### Bug Fixes

* **release:** drop the windows build that blocked binary uploads ([69d857b](https://github.com/bluegardenproject/tracks/commit/69d857b41719d6e408ec16c5bdf186f083fbc843))

## [0.4.0](https://github.com/bluegardenproject/tracks/compare/v0.3.1...v0.4.0) (2026-08-05)


### Features

* **claude:** skip pre-push review for docs-only diffs ([adc019d](https://github.com/bluegardenproject/tracks/commit/adc019dae9a60c259e86b94dd840bb1cce9dcaf8))
* **cmd:** reopen the tracks interrupted by the last shutdown ([db6936c](https://github.com/bluegardenproject/tracks/commit/db6936cc6c213eeed4b7baec54f0dffb9851967c))
* **daemon:** record interrupted tracks instead of erroring them ([3a711d6](https://github.com/bluegardenproject/tracks/commit/3a711d68518957596b000c74488c96462cf56c7b))
* **newtrack:** add doc-review track type ([866df94](https://github.com/bluegardenproject/tracks/commit/866df94bbc4f087c37509830113bbf6e310ae194))


### Documentation

* **roadmap:** log the test-spawns-into-live-tmux bug as high prio ([1d0bb67](https://github.com/bluegardenproject/tracks/commit/1d0bb671fe35b957dc8244c6d7d9a853badac103))

## [0.3.1](https://github.com/bluegardenproject/tracks/compare/v0.3.0...v0.3.1) (2026-07-21)


### Miscellaneous

* **main:** release 0.3.0 ([5cabeba](https://github.com/bluegardenproject/tracks/commit/5cabebaf50d614507f7095769d50c7a9e4635943))

## [0.3.0](https://github.com/bluegardenproject/tracks/compare/v0.2.0...v0.3.0) (2026-07-21)


### Features

* **claude:** add response-style and code-comment guidance to task prompt ([3022ceb](https://github.com/bluegardenproject/tracks/commit/3022ceb10d01fe905923360b45ab59db4ab6aa68))
* save a failed track creation as a draft ([793a777](https://github.com/bluegardenproject/tracks/commit/793a7771e7112b9b74c1696fc24dc46f284ff49d))
* **services:** run dev servers in a pane that owns the process ([466e05d](https://github.com/bluegardenproject/tracks/commit/466e05d58f9fc3eda5ad46b52f5ced8a92c2d611))
* **services:** start-all `tracks up` + live Proxy dashboard tab ([459281a](https://github.com/bluegardenproject/tracks/commit/459281aba51d4ddbea8555c9ca275a38b7de2cd1))


### Bug Fixes

* **dashboard:** keep track selection visible and stable ([487f703](https://github.com/bluegardenproject/tracks/commit/487f70376b12cfbc993b83ba8d087799f2fbffbc))
* **draft:** don't let End/Kill destroy a saved draft ([e5aa8de](https://github.com/bluegardenproject/tracks/commit/e5aa8de1aaef5ef5815b32cd29189459b611e2b8))
* never GC a worktree with unsaved work ([d551f80](https://github.com/bluegardenproject/tracks/commit/d551f800ed53e7adf50beefad25de287678920c5))
* **proxy:** bind stable-port proxy lazily so idle daemon frees the port ([3850624](https://github.com/bluegardenproject/tracks/commit/38506240fd9ed810c682b5ccd7be9a28767cb5b6))
* **services:** make the dev-server trigger reliable from inside a track ([b518a3a](https://github.com/bluegardenproject/tracks/commit/b518a3a7e6e22e9ac65502cebf819f31a21c1689))
* **test:** isolate StateDir in daemon server tests ([b3488e7](https://github.com/bluegardenproject/tracks/commit/b3488e7a0b5de95c1524facf991cb2f7b1267366))


### Documentation

* **roadmap:** reconcile with shipped work; record draft-on-failure ([88f2722](https://github.com/bluegardenproject/tracks/commit/88f27228edf25ffbc489c573911139f5aee5a90b))

## [0.2.0](https://github.com/bluegardenproject/tracks/compare/v0.1.0...v0.2.0) (2026-07-11)


### Features

* add install and uninstall scripts ([90df79d](https://github.com/bluegardenproject/tracks/commit/90df79d4814c33b2dec5855ad30b78386400ea64))
* **dashboard:** replace IDLE column with SVC, shrink BRANCH ([5aa0aef](https://github.com/bluegardenproject/tracks/commit/5aa0aef8b3f82ff44f8b5b1efc3c89d73a79ec92))
* resume finished tracks from their Claude session ([ac7ed74](https://github.com/bluegardenproject/tracks/commit/ac7ed74b8698024481df2b4fa0ba7b66fb59eb9f))
* **settings:** add per-repo submenu and service CRUD ([8622d01](https://github.com/bluegardenproject/tracks/commit/8622d014160728182d77fc565a6f850309d3466a))
* **status:** flip to PR as soon as PR URL appears in pane ([ddfe201](https://github.com/bluegardenproject/tracks/commit/ddfe20175b483d373190520f5d985a583f88ad79))


### Bug Fixes

* **dashboard:** make R (resume) keybinding async ([b68bdfd](https://github.com/bluegardenproject/tracks/commit/b68bdfd61dade2a7223a1e74acf8cc989094f496))
* resolve tmux pane, proxy menu, dep timing, and SVC column bugs ([b86548a](https://github.com/bluegardenproject/tracks/commit/b86548ae5441f845c5a5bc38f460431c6e259976))


### Documentation

* document curl install and tick roadmap distribution items ([eff7c80](https://github.com/bluegardenproject/tracks/commit/eff7c805d4981d494e96e1a1fab55a46c7278e19))
