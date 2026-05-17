# Reproducible demo recording for docs/demo.gif.
#
# Usage:
#   brew install vhs   # https://github.com/charmbracelet/vhs
#   make build
#   make demo-gif      # or: vhs demo.tape
#
# Output:
#   docs/demo.gif      (referenced by README.md)
#
# The dimensions are sized so the TUI's adaptive panels all fit:
#   - Session Stats column (gated at >=100 cols)
#   - NVLink Status column (gated at >=100 cols)
#   - GPU History "fire-bar" charts at the bottom
#     (Util / VRAM / Temp / Power / Clock)
# At FontSize 14 these dimensions give roughly 166 cols x 58 rows.
#
# This tape walks through every panel and every keybind so the GIF
# doubles as a feature tour:
#   GPU panel       -> j/k cursor, throttle / parked / PCIe badges
#   Process panel   -> sort cycle (Util% -> VRAM -> PID)
#   Pause / resume  -> space toggle
#   Charts panel    -> 5 stacked metrics, z to cycle 2m / 5m / 10m
#   Hints panel     -> open detail popups for several rules
#   Hint history    -> the same panel after switching with H
#   Help overlay    -> ? to show, ? again to dismiss
#   Quit            -> q

Output docs/demo.gif

Set FontSize 14
Set Width 1400
Set Height 1000
Set Padding 20
Set Theme "Dracula"
Set TypingSpeed 20ms
Set PlaybackSpeed 2.0

Type "./bin/gputui --demo --refresh 1s"
Enter

# Let the sparkline + chart history fill so every panel has data
# and the hysteresis-wrapped hints have settled into steady state.
Sleep 8s

# --- GPU panel ---
# Show that the GPU table is the default focus, then move the cursor
# down so viewers see the row highlight, the THROTTLE badge on the
# stressed GPU, the [P8] parked badge, and the PCIe degraded row.
Type "g"
Sleep 800ms
Type "j"
Sleep 500ms
Type "j"
Sleep 500ms
Type "j"
Sleep 500ms
Type "j"
Sleep 500ms
Type "j"
Sleep 500ms
Type "k"
Sleep 500ms
Type "k"
Sleep 500ms

# --- Process panel + sort cycle ---
# Cycle the process-table sort: Util% -> VRAM -> PID -> back to Util%.
Type "p"
Sleep 800ms
Type "s"
Sleep 1500ms
Type "s"
Sleep 1500ms
Type "s"
Sleep 1500ms

# --- Pause / resume ---
# Pause the refresh loop so viewers see the PAUSED badge in the header,
# then resume.
Type " "
Sleep 1500ms
Type " "
Sleep 800ms

# --- Charts panel + zoom ---
# Focus the GPU History charts at the bottom and cycle the zoom levels
# 5min (default) -> 10min -> 2min -> 5min.
Type "c"
Sleep 1500ms
Type "z"
Sleep 1200ms
Type "z"
Sleep 1200ms
Type "z"
Sleep 1200ms

# --- Hints panel ---
# Focus Active Hints, walk into the first hint and open its detail
# popup so viewers see the description + evidence (metrics + thresholds
# that triggered it). Then move to a couple more hints in different
# categories so different evidence types are shown.
Type "h"
Sleep 800ms

# 1st hint: usually the critical confirmed-throttle.
Enter
Sleep 4s
Enter
Sleep 600ms

# 2nd hint: gpu-parked or memory-bandwidth-bound.
Type "j"
Sleep 500ms
Enter
Sleep 4s
Enter
Sleep 600ms

# 3rd hint: pcie-link-degraded.
Type "j"
Sleep 500ms
Enter
Sleep 4s
Enter
Sleep 600ms

# 4th hint: thermal-violation-outlier or nvlink-health.
Type "j"
Sleep 500ms
Enter
Sleep 4s
Enter
Sleep 800ms

# --- Hint history panel ---
# Switch to the hint history view so viewers see the (active vs
# previously-seen) presentation.
Type "H"
Sleep 2500ms

# --- Help overlay ---
# Show the help overlay so viewers learn the keybinds, then dismiss.
Type "?"
Sleep 3500ms
Type "?"
Sleep 800ms

Type "q"
