"""Disposable-home recovery acceptance; never invokes a package manager."""
import os, pty, fcntl, termios, struct, select, time, codecs, json, pathlib, tempfile, signal, hashlib, subprocess, stat
import pyte
ROOT = pathlib.Path(__file__).resolve().parents[5]
OUT = pathlib.Path(__file__).resolve().parent
BIN = pathlib.Path('/tmp/dotfiles-release-recovery')
home = pathlib.Path(tempfile.mkdtemp(prefix='dotfiles-recovery-acceptance-'))
os.chmod(home, 0o700)
empty = home/'empty-bin'; empty.mkdir(mode=0o700)
env = {'HOME':str(home), 'XDG_CONFIG_HOME':str(home/'.config'), 'XDG_STATE_HOME':str(home/'.local/state'), 'PATH':str(empty), 'TERM':'xterm-256color', 'COLORTERM':'truecolor', 'LANG':'en_US.UTF-8'}
checks=[]
def write(path, data, mode=0o600):
 path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
 path.write_bytes(data);path.chmod(mode)
def snapshot(path):
 return {str(p.relative_to(path)):{'sha256':hashlib.sha256(p.read_bytes()).hexdigest(),'mode':oct(stat.S_IMODE(p.stat().st_mode))} for p in sorted(path.rglob('*')) if p.is_file() and not p.is_symlink()}
def cli(name, *args):
 result=subprocess.run([str(BIN),*args],env=env,capture_output=True,timeout=20)
 (OUT/(name+'.stdout')).write_bytes(result.stdout);(OUT/(name+'.stderr')).write_bytes(result.stderr)
 return result
originals={'.zshrc':(b'export QA_ORIGINAL=1\n',0o600),'.tmux.conf':(b'set -g status-position top\n',0o640),'.config/nvim/init.lua':(b'vim.opt.number = true\n',0o600),'.config/ghostty/config':(b'font-size = 13\n',0o640),'.gitconfig':(b'[user]\n name = Synthetic QA\n',0o600),'.config/yazi/yazi.toml':(b'[manager]\nratio = [1, 4, 3]\n',0o600),'.config/yazi/keymap.toml':(b'# synthetic keymap\n',0o600),'.config/yazi/theme.toml':(b'# synthetic theme\n',0o600)}
for rel,(data,mode) in originals.items():write(home/rel,data,mode)
pid,fd=pty.fork()
if pid==0:os.execve(str(BIN),['dotfiles','--skip-intro','manage'],env)
fcntl.ioctl(fd,termios.TIOCSWINSZ,struct.pack('HHHH',24,80,0,0))
screen=pyte.Screen(80,24);stream=pyte.Stream(screen);decoder=codecs.getincrementaldecoder('utf-8')('replace')
def read(seconds=.6):
 deadline=time.monotonic()+seconds
 while time.monotonic()<deadline:
  ready,_,_=select.select([fd],[],[],.05)
  if ready:
   try:data=os.read(fd,65536)
   except OSError:return
   if not data:return
   with (OUT/'pty.raw.log').open('ab') as log:log.write(data)
   if b'\x1b[6n' in data:os.write(fd,b'\x1b[1;1R')
   if b'\x1b]11;?' in data:os.write(fd,b'\x1b]11;rgb:0000/0000/0000\x1b\\')
   stream.feed(decoder.decode(data))
def key(value,delay=.6):os.write(fd,value);read(delay)
def expect(text,name):
 value='\n'.join(screen.display);(OUT/(name+'.screen.txt')).write_text(value);assert text in value,(name,text,value)
try:
 read(2);expect('TOOLS','01-tools')
 for keyval,label,name in [(b'2','User Profiles','02-users'),(b'3','Cheatsheets','03-hotkeys'),(b'4','Package Updates','04-updates'),(b'5','Backups','05-backups')]:key(keyval);expect(label,name)
 key(b'n',1.5);expect('1 backup','06-created')
 backups=home/'.config/dotfiles/backups'; sessions=list(backups.iterdir());assert len(sessions)==1
 backup=sessions[0]; before=snapshot(backups)
 manifest=(backup/'manifest.txt').read_text();assert all(rel in manifest for rel in originals),manifest
 checks.append('TUI convenience backup captures eight real synthetic config files in manifest')
 for rel in originals:write(home/rel,b'# changed after backup\n',0o644)
 key(b'\r');expect('Restore backup','07-confirm');key(b'n')
 assert all((home/rel).read_bytes()==b'# changed after backup\n' for rel in originals)
 checks.append('TUI restore cancellation preserves all modified files')
 key(b'\r');key(b'y',1.5);expect('Backups','08-restored')
 for rel,(data,mode) in originals.items():assert (home/rel).read_bytes()==data and stat.S_IMODE((home/rel).stat().st_mode)==mode,rel
 assert snapshot(backups)==before
 checks.append('TUI confirmed restore recovers all eight bytes and POSIX modes; backup payload unchanged')
 key(b'd');expect('Delete backup','09-delete-confirm');key(b'n');assert snapshot(backups)==before
 checks.append('TUI delete cancellation preserves full backup')
 for rows,cols in [(14,40),(32,100),(24,80)]:
  fcntl.ioctl(fd,termios.TIOCSWINSZ,struct.pack('HHHH',rows,cols,0,0));screen.resize(lines=rows,columns=cols);read(.7);expect('enter restore' if cols==40 else 'Backups',f'10-resize-{cols}x{rows}')
 checks.append('Restore action remains visible at 40x14; Backups heading returns at 100x32 and 80x24')
finally:
 os.write(fd,b'\x03');read(.3)
 deadline=time.monotonic()+5
 while True:
  waited,status=os.waitpid(pid,os.WNOHANG)
  if waited:break
  if time.monotonic()>deadline:os.kill(pid,signal.SIGKILL);os.waitpid(pid,0);raise RuntimeError('TUI failed to exit')
  time.sleep(.05)
 os.close(fd)
assert os.waitstatus_to_exitcode(status)==0
checks.append('TUI Ctrl-C exits zero')
for rel in originals:write(home/rel,b'# CLI modified\n',0o644)
r=cli('11-cli-restore','restore',backup.name);assert r.returncode==0,(r.stdout,r.stderr)
for rel,(data,mode) in originals.items():assert (home/rel).read_bytes()==data and stat.S_IMODE((home/rel).stat().st_mode)==mode,rel
checks.append('CLI restores TUI-created backup: eight file bytes and modes')
write(home/'.zshrc',b'# changed before uninstall\n')
r=cli('12-uninstall','uninstall','--force');assert r.returncode==0,(r.stdout,r.stderr)
assert (home/'.zshrc').read_bytes()==originals['.zshrc'][0] and snapshot(backups)==before
checks.append('CLI uninstall restores latest backup and retains complete backup/config state')
# Restore refuses a live symlink while preserving its target and recovery payload.
external=home/'synthetic-external';write(external,b'UNCHANGED EXTERNAL\n')
(home/'.zshrc').unlink();(home/'.zshrc').symlink_to(external)
r=cli('13-symlink-refusal','restore',backup.name);assert r.returncode!=0
assert external.read_bytes()==b'UNCHANGED EXTERNAL\n' and (home/'.zshrc').is_symlink() and snapshot(backups)==before
checks.append('CLI reports partial failure for symlink destination; target and recovery data preserved')
(home/'.zshrc').unlink();write(home/'.zshrc',b'# live after failed restore\n')
# A catalogued backup missing a payload must report failure without deleting backups.
(backup/'.zshrc').unlink()
corrupt_snapshot=snapshot(backups)
r=cli('14-incomplete-uninstall','uninstall','--force');assert r.returncode!=0,(r.stdout,r.stderr)
assert snapshot(backups)==corrupt_snapshot and (home/'.zshrc').read_bytes()==b'# live after failed restore\n'
checks.append('Incomplete backup makes uninstall fail; available recovery data and unrestorable live file survive')
# Legacy directory manifest and explicit absent-before records exercise actual CLI.
for p in list(backups.iterdir()):p.rename(home/('archived-'+p.name))
legacy=backups/'legacy-directory';legacy.mkdir(mode=0o700)
write(legacy/'.config/nvim/init.lua',b'original nested init\n')
write(legacy/'.config/nvim/lua/plugin.lua',b'original nested plugin\n',0o640)
write(home/'.config/nvim/extra.lua',b'created after backup\n')
write(home/'.config/generated/new',b'created after backup\n')
text=f"{home}/.config/nvim|{legacy}/.config/nvim|yes|directory|0700\n{home}/.config/generated||no|directory|0700\n"
write(legacy/'manifest.txt',text.encode())
r=cli('15-legacy-directory','restore',legacy.name);assert r.returncode==0,(r.stdout,r.stderr)
assert (home/'.config/nvim/init.lua').read_bytes()==b'original nested init\n'
assert (home/'.config/nvim/lua/plugin.lua').read_bytes()==b'original nested plugin\n'
assert stat.S_IMODE((home/'.config/nvim/lua/plugin.lua').stat().st_mode)==0o640
assert not (home/'.config/nvim/extra.lua').exists() and not (home/'.config/generated').exists()
checks.append('Legacy directory CLI restore recovers nested bytes/modes, removes extra descendants and absent-before directory')
report={'result':'passed','binary_sha256':hashlib.sha256(BIN.read_bytes()).hexdigest(),'platform':os.uname().sysname+' '+os.uname().machine,'checks':checks,'isolation':'Private fresh HOME/XDG_CONFIG_HOME/XDG_STATE_HOME; empty PATH; synthetic config only','limits':['At 40x14 only Backups details/footer are visible; full listing/header are clipped','No owner account config or package mutations','No macOS VM/spare-Mac disk snapshot recovery; required owner install/apply gate remains unfulfilled','PTY emulation does not verify GUI-terminal font, mouse, animation or physical appearance','No ACL, xattr, file flags, hardlink topology or timestamps preservation claim','No Arch or Raspberry Pi runtime covered by this harness'],'synthetic_home':str(home)}
(OUT/'acceptance.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(report,indent=2))
