# Compatibility

LapGuard auto-discovers what a Linux laptop actually exposes. Features are enabled
only when the corresponding sysfs files, kernel modules, or userspace tools are
present. Missing hardware is not an error: the capability is marked `none` with a
`why_not` explanation.

Charge-threshold method preference:

1. **sysfs** — `charge_control_start_threshold` / `charge_control_end_threshold`
   (or the older ThinkPad `charge_start_threshold` / `charge_stop_threshold` names,
   including `/sys/devices/platform/thinkpad_acpi/`)
2. **tlp** — `tlp-stat` reports an active start/stop backend; writes use
   `tlp setcharge START STOP BATX`
3. **none** — no writable firmware limit

## Tested machines

| Machine | Role | Battery sysfs | Charge thresholds | Notes |
| --- | --- | --- | --- | --- |
| Fujitsu Lifebook | Production target | `energy_*`, `power_now`, `cycle_count`, `temp` | **none** | `fujitsu_laptop` loads but does not register charge control (dmesg: *Unable to register battery charge control*). TLP may be installed and still cannot set limits. |
| HP ProDesk | Development box | no pack | **none** | No `/sys/class/power_supply/BAT*`. Provider `auto` falls back to mock telemetry. Discovery still reports host modules/tools. |
| ThinkPad (typical, mocked) | Profile | `energy_*` plus `charge_control_*` | **sysfs** (TLP also present) | `thinkpad_acpi` (+ `acpi_call` on some generations). If sysfs charge control is absent but TLP NATACPI is active, method is **tlp**. |
| Dell XPS (typical, mocked) | Profile | `charge_*` plus `charge_control_*` | **sysfs** | `dell_wmi` / `dell_smbios`. TLP may also be installed; sysfs wins. |
| ASUS notebook (typical, mocked) | Profile | `charge_*`, often end-threshold only | **sysfs** | `asus_wmi` / `asus_nb_wmi`. Many models only expose `charge_control_end_threshold`. |
| Generic ACPI battery (mocked) | Profile | `energy_*` or `charge_*`, no vendor extras | **none** | Power from `power_now` or `current_now × voltage_now`. Telemetry only. |

## Kernel modules LapGuard looks for

| Module | What it enables |
| --- | --- |
| `fujitsu_laptop` | Fujitsu extras. Charge control often **fails to register**. |
| `thinkpad_acpi` | ThinkPad extras; charge_control sysfs on modern kernels. |
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

1. Run LapGuard on the machine and open `GET /api/v1/discover`.
2. Confirm `features.charge_thresholds` is `sysfs`, `tlp`, or `none`.
3. Record the battery naming convention (`energy` / `charge` / `both`) and any vendor module.
4. Send a PR updating this table. Mock profiles for CI live in `internal/discovery/laptops_test.go`.
