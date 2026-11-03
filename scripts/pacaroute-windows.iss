; Compile on Windows using Inno Setup 6 after building the portable desktop binaries.
#ifndef SourceDir
  #define SourceDir "..\artifacts\pacaroute-desktop-release\windows-amd64"
#endif
[Setup]
AppId={{CD276767-FCF8-4744-9C51-4DE22E89ED02}
AppName=PacaRoute
AppVersion=0.8.0
DefaultDirName={localappdata}\Programs\PacaRoute
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputBaseFilename=PacaRoute-Windows-x64-Setup
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
SetupIconFile=..\internal\pacaroute\trayassets\mark.ico
UninstallDisplayIcon={app}\PacaRoute.ico
[Files]
Source: "{#SourceDir}\PacaRoute.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\pacaroute-cli.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\PacaRoute.ico"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\THIRD_PARTY_NOTICES.md"; DestDir: "{app}"
Source: "{#SourceDir}\README.md"; DestDir: "{app}"
[Icons]
Name: "{autoprograms}\PacaRoute"; Filename: "{app}\PacaRoute.exe"; IconFilename: "{app}\PacaRoute.ico"
[Run]
Filename: "{app}\PacaRoute.exe"; Description: "Launch PacaRoute"; Flags: nowait postinstall skipifsilent
[UninstallDelete]
Type: files; Name: "{userstartup}\PacaRoute.lnk"
[Registry]
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueName: "PacaRoute"; Flags: uninsdeletevalue
