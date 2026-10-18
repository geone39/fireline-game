.PHONY: check content econ sim
check: content econ sim
content: ; python3 tools/validate_content.py
econ:    ; python3 tools/econ_sim.py > /dev/null
sim:     ; cd sim && go test ./...
