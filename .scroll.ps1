param([int]$X = 700, [int]$Y = 400, [int]$Notches = -5)

Add-Type @'
using System;
using System.Runtime.InteropServices;
public class S {
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int x, int y);
  [DllImport("user32.dll")] public static extern void mouse_event(uint f, uint x, uint y, uint d, IntPtr e);
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out R r);
  [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, IntPtr pid);
  [DllImport("user32.dll")] public static extern bool AttachThreadInput(uint a, uint b, bool attach);
  [DllImport("kernel32.dll")] public static extern uint GetCurrentThreadId();
  [StructLayout(LayoutKind.Sequential)] public struct R { public int L, T, Rt, B; }
}
'@

$p = Get-Process hyphp-dbg | Where-Object { $_.MainWindowHandle -ne 0 } | Select-Object -First 1
$h = $p.MainWindowHandle
$fg = [S]::GetForegroundWindow()
$tidFg = [S]::GetWindowThreadProcessId($fg, [IntPtr]::Zero)
[void][S]::AttachThreadInput([S]::GetCurrentThreadId(), $tidFg, $true)
[void][S]::SetForegroundWindow($h)
[void][S]::AttachThreadInput([S]::GetCurrentThreadId(), $tidFg, $false)
Start-Sleep -Milliseconds 400

[S+R]$r = New-Object S+R
[void][S]::GetWindowRect($h, [ref]$r)
[void][S]::SetCursorPos(($r.L + $X), ($r.T + $Y))
Start-Sleep -Milliseconds 150
# WHEEL_DELTA = 120 por entalhe; negativo rola para baixo. O parametro e
# UINT, entao o valor negativo vai como complemento de dois.
# 0xFFFFFFFF sem sufixo vira Int32 (-1) no PowerShell e a mascara nao faz nada.
$delta = [uint32]([int64]($Notches * 120) -band 0xFFFFFFFFL)
[S]::mouse_event(0x0800, 0, 0, $delta, [IntPtr]::Zero)
Start-Sleep -Milliseconds 700
Write-Output "scroll $Notches"
