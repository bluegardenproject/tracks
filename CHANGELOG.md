# Changelog

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
