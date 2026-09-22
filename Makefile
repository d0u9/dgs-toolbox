BINDIR     ?= $(HOME)/.local/bin
DATADIR    ?= $(HOME)/.local/share
ZSHCOMPDIR ?= $(DATADIR)/zsh/site-functions
BASHCOMPDIR ?= $(DATADIR)/bash-completion/completions
BIN        := dgs
COMPLETION ?=

.PHONY: install install-zsh install-bash uninstall

# On macOS dgs links EventKit through cgo for the reminder Actions, and carries
# an Info.plist so Reminders can say who is asking. It needs only the Command
# Line Tools; elsewhere, or with CGO_ENABLED=0, the reminder Actions refuse.
DARWIN_LDFLAGS := -linkmode=external -extldflags "-sectcreate __TEXT __info_plist $(CURDIR)/internal/desktop/reminders/Info.plist"

install-zsh: COMPLETION=zsh
install-zsh: install

install-bash: COMPLETION=bash
install-bash: install

install:
	@completion="$(COMPLETION)"; \
	if [ -z "$$completion" ]; then completion="$${SHELL##*/}"; fi; \
	case "$$completion" in \
		zsh|bash) ;; \
		*) echo "cannot install completion for '$$completion': use make install-zsh or make install-bash" >&2; exit 2;; \
	esac
	@mkdir -p "$(BINDIR)"
	@if [ "$$(uname)" = Darwin ]; then \
		go build -ldflags '$(DARWIN_LDFLAGS)' -o "$(BINDIR)/$(BIN)" ./cmd/dgs && \
		codesign --force --sign - --identifier dev.dgs-toolbox.dgs "$(BINDIR)/$(BIN)"; \
	else go build -o "$(BINDIR)/$(BIN)" ./cmd/dgs; fi
	@rm -f "$(BINDIR)/dgs-reminders" # the helper reminders needed before they moved into dgs
	@echo "installed $(BINDIR)/$(BIN)"
	@completion="$(COMPLETION)"; \
	if [ -z "$$completion" ]; then completion="$${SHELL##*/}"; fi; \
	case "$$completion" in \
		zsh) \
			mkdir -p "$(ZSHCOMPDIR)"; \
			"$(BINDIR)/$(BIN)" completion zsh > "$(ZSHCOMPDIR)/_$(BIN)"; \
			echo "installed $(ZSHCOMPDIR)/_$(BIN)"; \
			echo "note: zsh caches completions; to use the new ones now, run"; \
			echo "        rm -f ~/.cache/zsh/zcompdump-*(N) ~/.zcompdump*(N)"; \
			echo "      and open a new terminal";; \
		bash) \
			mkdir -p "$(BASHCOMPDIR)"; \
			"$(BINDIR)/$(BIN)" completion bash > "$(BASHCOMPDIR)/$(BIN)"; \
			echo "installed $(BASHCOMPDIR)/$(BIN)";; \
	esac
	@case ":$$PATH:" in *":$(BINDIR):"*) ;; *) echo "note: $(BINDIR) is not on your PATH";; esac

uninstall:
	@for f in "$(BINDIR)/$(BIN)" "$(ZSHCOMPDIR)/_$(BIN)" "$(BASHCOMPDIR)/$(BIN)"; do \
		if [ -e "$$f" ]; then rm -f "$$f" && echo "removed $$f"; \
		else echo "$$f is not installed"; fi; \
	done
