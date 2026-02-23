!define ROOT_DIR "${__FILEDIR__}\.."

OutFile "KeenBench-Setup.exe"
InstallDir "$PROGRAMFILES\KeenBench"

Section "Install"
  SetOutPath "$INSTDIR"
  File /r "${ROOT_DIR}\app\build\windows\x64\runner\Release\*.*"
  CreateShortCut "$DESKTOP\KeenBench.lnk" "$INSTDIR\keenbench.exe"
SectionEnd
