BINDIR     ?= $(HOME)/.local/bin
DATADIR    ?= $(HOME)/.local/share
ZSHCOMPDIR ?= $(DATADIR)/zsh/site-functions
BASHCOMPDIR ?= $(DATADIR)/bash-completion/completions
BIN        := dgs
HELPER     := dgs-reminders

.PHONY: install uninstall reminders

# dgs-reminders is the EventKit helper behind apple.reminders.create. It needs
# only the Command Line Tools; on anything but macOS it is not built.
reminders:
	@mkdir -p bin
	swiftc -O -swift-version 5 helpers/reminders/main.swift -o bin/$(HELPER) \
		-Xlinker -sectcreate -Xlinker __TEXT -Xlinker __info_plist -Xlinker helpers/reminders/Info.plist
	codesign --force --sign - --identifier dev.dgs-toolbox.reminders bin/$(HELPER)

install:
	@mkdir -p "$(BINDIR)" "$(ZSHCOMPDIR)" "$(BASHCOMPDIR)"
	go build -o "$(BINDIR)/$(BIN)" ./cmd/dgs
	@if [ "$$(uname)" = Darwin ]; then $(MAKE) reminders && install -m 755 bin/$(HELPER) "$(BINDIR)/$(HELPER)" && echo "installed $(BINDIR)/$(HELPER)"; fi
	"$(BINDIR)/$(BIN)" completion zsh > "$(ZSHCOMPDIR)/_$(BIN)"
	"$(BINDIR)/$(BIN)" completion bash > "$(BASHCOMPDIR)/$(BIN)"
	@echo "installed $(BINDIR)/$(BIN)"
	@echo "installed $(ZSHCOMPDIR)/_$(BIN)"
	@echo "installed $(BASHCOMPDIR)/$(BIN)"
	@case ":$$PATH:" in *":$(BINDIR):"*) ;; *) echo "note: $(BINDIR) is not on your PATH";; esac
	@echo "note: zsh caches completions; to use the new ones now, run"
	@echo "        rm -f ~/.cache/zsh/zcompdump-* ~/.zcompdump*"
	@echo "      and open a new terminal"

uninstall:
	@for f in "$(BINDIR)/$(BIN)" "$(BINDIR)/$(HELPER)" "$(ZSHCOMPDIR)/_$(BIN)" "$(BASHCOMPDIR)/$(BIN)"; do \
		if [ -e "$$f" ]; then rm -f "$$f" && echo "removed $$f"; \
		else echo "$$f is not installed"; fi; \
	done
