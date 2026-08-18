# Compatibility

LapGuard auto-discovers what a Linux laptop actually exposes. Features are enabled
only when the corresponding sysfs files, kernel modules, or userspace tools are
present. Missing hardware is not an error: the capability is marked `none` with a
`why_not` explanation.

Charge-threshold **detection** preference (LapGuard does **not** write limits
in this alpha):

1. **sysfs** — `charge_control_start_threshold` / `charge_control_end_threshold`
   (or the older ThinkPad `charge_start_threshold` / `charge_stop_threshold` names,
   including `/sys/devices/platform/thinkpad_acpi/`)
2. **tlp** — `tlp-stat` reports an active start/stop backend (TLP itself would
   write with `tlp setcharge START STOP BATX`; LapGuard only detects this)
3. **none** — no writable firmware limit

## Power readings

LapGuard reports **battery-side power**, not total laptop/system consumption.

| Flag | Meaning |
| --- | --- |
| `raw_power_now_supported` | `/sys/class/power_supply/BAT*/power_now` exists |
| `derived_power_supported` | `current_now` and `voltage_now` exist, so watts = current × voltage |

On a **Gigabyte Aero 16**, status `Charging` at 5% with derived
`current_now × voltage_now` was about **36 W**. That value is **charging power
into the battery**, not what the whole machine was drawing from the wall.

| Pack state | What the watts mean |
| --- | --- |
| Charging | Power flowing into the battery |
| Discharging | Power drawn from the battery |
| Full / Not charging with `current_now=0` | **0 W** (idle) — a valid reading, not a missing sensor |

Telemetry fields:

- `power_w` — kept for compatibility. Signed: negative while discharging,
  positive while charging. Interpret it together with `power_direction`.
- `battery_power_w` — the same magnitude, always ≥ 0
- `power_direction` — `charge` \| `discharge` \| `idle` \| `unknown`
- `power_label` — `Battery charging power` / `Battery discharge power` /
  `Battery idle` / `Power unavailable`

The dashboard uses those labels. It does **not** call charging watts
“instantaneous laptop power” or “power consumption”. When the value is derived,
the UI still shows the `current_now × voltage_now` formula.

Estimated runtime stays available **only while discharging**. It is never
computed while charging or connected to AC.

## AC adapters

Mains supplies are discovered by `type=Mains` and the `online` attribute. Names such as `AC`, `ACAD`, `ADP1`, or `AC0` are not hardcoded. `power_loss_detection` is true when at least one mains `online` file is readable.

## Tested machines

Full field lists for verified machines are in the sections below this table.

| Machine | Role | Battery sysfs | Charge thresholds | Notes |
| --- | --- | --- | --- | --- |
| Fujitsu Lifebook A3510 | Production target (verified) | **BAT1**, naming `charge` | **none** | No `power_now`, `temp`, or `charge_control_*`. Derived power. TLP cannot set thresholds. See below. |
| Gigabyte Aero 16 | Verified on Zorin OS 18.1 | **BAT1**, `charge_*` with `current_now` and `voltage_now`; no `power_now` or `temp` | **none** | `asus_wmi` is loaded and TLP 1.6.1 is detected, but no writable charge-control attributes are exposed. Derived battery-side power and estimated runtime work. |
| HP ProDesk | Development box | no pack | **none** | No `BAT*` supply. Provider `auto` falls back to mock telemetry. |
| ThinkPad (typical, mocked) | Profile | `energy_*` plus `charge_control_*` | **sysfs** (TLP also present) | `thinkpad_acpi`. Method is **tlp** if sysfs charge control is missing and TLP NATACPI is active. |
| Dell XPS (typical, mocked) | Profile | `charge_*` plus `charge_control_*` | **sysfs** | `dell_wmi` / `dell_smbios`. Often no `power_now`; derived power from current × voltage. |
| ASUS notebook (typical, mocked) | Profile | `charge_*`, often end-threshold only | **sysfs** | `asus_wmi` / `asus_nb_wmi`. Many models only expose `charge_control_end_threshold`. |
| Generic ACPI battery (mocked) | Profile | `energy_*` or `charge_*` | **none** | Power from `power_now` or derived `current_now × voltage_now`. |

## Fujitsu Lifebook A3510 (verified)

Real pack on **BAT1**. Naming convention is **charge**, not `energy_*`. This machine does **not** expose `power_now` or `temp`.

Available sysfs fields:

- `charge_now`, `charge_full`, `charge_full_design`
- `current_now`, `voltage_now`
- `cycle_count`, `alarm`
- `capacity`, `capacity_level`, `present`, `status`
- `manufacturer`, `model_name`, `technology`

Not available:

- `power_now`
- `temp`
- `charge_control_start_threshold`
- `charge_control_end_threshold`

Verified capabilities:

- Battery name: **BAT1**
- `naming_convention`: `charge`
- `raw_power_now_supported`: **false**
- `derived_power_supported`: **true** — battery-side watts = `current_now × voltage_now`
- **0 W** is a valid reading when `current_now` is zero (idle / full), not a missing sensor
- Watts are pack charge/discharge power, not total system consumption
- LapGuard derives energy (Wh) from charge (Ah) × voltage where those fields exist (`charge_*` × `voltage_now`)
- Charge-threshold method: **none**. `fujitsu_laptop` is loaded but does not register charge control (typical dmesg: *Unable to register battery charge control*). TLP is detected and still reports `tlp_can_set_thresholds=false`.

Do not record battery serial numbers, webhook URLs, tokens, passwords, usernames, or IP addresses in this file.

## Gigabyte Aero 16 (verified)

Verified on **Zorin OS 18.1**, kernel **6.17.0-35-generic**. Real pack on **BAT1**. Naming convention is **charge**, not `energy_*`. This machine does **not** expose `power_now` or `temp`.

Available sysfs fields:

- `charge_now`, `charge_full`, `charge_full_design`
- `current_now`, `voltage_now`
- `cycle_count`, `alarm`
- `capacity`, `capacity_level`, `present`, `status`
- `manufacturer`, `model_name`, `technology`

Not available:

- `power_now`
- `temp`
- `charge_control_start_threshold`
- `charge_control_end_threshold`

Verified capabilities:

- Battery name: **BAT1**
- `naming_convention`: `charge`
- `raw_power_now_supported`: **false**
- `derived_power_supported`: **true** — power calculation is `current_now * voltage_now`
- Watts are pack charge/discharge power, not total system consumption
- Derived battery-side power and estimated runtime work. Estimated runtime is
  available **only while discharging**, never while charging or on AC.
- LapGuard derives energy (Wh) from charge (Ah) × voltage where those fields exist (`charge_*` × `voltage_now`)
- Charge-threshold method: **none**. Charge-control sysfs attributes are not present, so thresholds are not writable on this tested Aero 16. That is a fact about this machine, not a claim that ASUS/Gigabyte charge limits are generally unsupported.
- `asus_wmi` is loaded
- TLP **1.6.1** is detected; `tlp_can_set_thresholds` is **false**
- AC adapter **ADP1** is discovered via `type=Mains` and `online` (not a hardcoded name)
- `power_loss_detection`: **true**
- Notifications were not configured in this test

`lapguard discover --report` does not start SQLite event storage or the safety controller. Absence of those flags in a compatibility export is not a hardware gap.

Do not record battery serial numbers, webhook URLs, tokens, passwords, usernames, or IP addresses in this file.

## Kernel modules LapGuard looks for

| Module | What it enables |
| --- | --- |
| `fujitsu_laptop` | Fujitsu extras. On the A3510 it loads but **does not** register charge control (no `charge_control_*` sysfs). |
| `thinkpad_acpi` | ThinkPad extras; sysfs charge-control attributes on modern kernels. |
| `dell_smbios`, `dell_wmi` | Dell SMBIOS/WMI extras. |
| `asus_wmi`, `asus_nb_wmi` | ASUS WMI; charge end-threshold on many models. |
| `hp_wmi`, `hp_accel` | HP extras / accelerometer. Charge limits are uncommon. |
| `tp_smapi` | Older ThinkPad SMAPI start/stop thresholds. |
| `acpi_call` | Raw ACPI methods used by TLP/`tpacpi-bat`. |

## Userspace tools

| Tool | Detection |
| --- | --- |
| TLP | `tlp` on `PATH`; version from `tlp --version` / `tlp-stat`; threshold support from `tlp-stat -b` |
| UPower | `upower` on `PATH` |
| ACPI | `acpi` on `PATH` |
| tp-smapi | `tp-smapi-cli` or `tpacpi-bat`, or the `tp_smapi` module |
| i8kutils (Dell) | `i8kctl` or `i8kfan` |

## How to add a laptop to this list

1. On the machine, run `lapguard discover --report > lapguard-compatibility-report.json` (add `--pretty` to indent).
2. Confirm `features.charge_thresholds` is `sysfs`, `tlp`, or `none`.
3. Record `raw_power_now_supported` vs `derived_power_supported` (absence of `power_now` is not a failure if current×voltage works). Record `power_direction` / `battery_power_w` if you are describing charging vs discharge watts.
4. Record the battery naming convention (`energy` / `charge` / `both`) and any vendor module. For charge-named packs, note that energy (Wh) is derived from charge × voltage.
5. **Privacy:** the CLI export already omits serial numbers, usernames, home paths, IP addresses, tokens, and webhook URLs. Do not paste `config.json` or unsanitized `GET /api/v1/discover` JSON.
6. Open a GitHub issue with the **Compatibility report** template and attach the JSON, or send a PR updating this table. Mock profiles for CI live in `internal/discovery/laptops_test.go`.
