BINDIR ?= $(HOME)/.local/bin
BIN    := dgs

.PHONY: install uninstall

install:
	@mkdir -p "$(BINDIR)"
	go build -o "$(BINDIR)/$(BIN)" ./cmd/dgs
	@echo "installed $(BINDIR)/$(BIN)"
	@case ":$$PATH:" in *":$(BINDIR):"*) ;; *) echo "note: $(BINDIR) is not on your PATH";; esac

uninstall:
	@if [ -e "$(BINDIR)/$(BIN)" ]; then rm -f "$(BINDIR)/$(BIN)" && echo "removed $(BINDIR)/$(BIN)"; \
	else echo "$(BINDIR)/$(BIN) is not installed"; fi
