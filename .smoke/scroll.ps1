param([int]$X=700,[int]$Y=450,[int]$N=-8)
Add-Type @'
using System;using System.Runtime.InteropServices;
public class S {
 [DllImport("user32.dll")] public static extern bool SetCursorPos(int x,int y);
 [DllImport("user32.dll")] public static extern void mouse_event(uint f,uint x,uint y,uint d,IntPtr e);
 [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
 [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out R r);
 [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
 [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, IntPtr pid);
 [DllImport("user32.dll")] public static extern bool AttachThreadInput(uint a,uint b,bool at);
 [DllImport("kernel32.dll")] public static extern uint GetCurrentThreadId();
 [StructLayout(LayoutKind.Sequential)] public struct R { public int L,T,Rt,B; }
}
'@
$p = Get-Process hyphp-dbg | Where-Object { $_.MainWindowHandle -ne 0 } | Select-Object -First 1
$h=$p.MainWindowHandle
$fg=[S]::GetForegroundWindow(); $t=[S]::GetWindowThreadProcessId($fg,[IntPtr]::Zero)
[void][S]::AttachThreadInput([S]::GetCurrentThreadId(),$t,$true)
[void][S]::SetForegroundWindow($h)
[void][S]::AttachThreadInput([S]::GetCurrentThreadId(),$t,$false)
Start-Sleep -Milliseconds 400
[S+R]$r = New-Object S+R
[void][S]::GetWindowRect($h,[ref]$r)
[void][S]::SetCursorPos(($r.L+$X),($r.T+$Y)); Start-Sleep -Milliseconds 150
$d=[uint32]([int64]($N*120) -band 0xFFFFFFFFL)
[S]::mouse_event(0x0800,0,0,$d,[IntPtr]::Zero); Start-Sleep -Milliseconds 700
