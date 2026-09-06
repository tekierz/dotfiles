# Requires Python 3 and pyte 0.8.2. Executes only bin/dotfiles in a new private HOME.
import os,pty,fcntl,termios,struct,select,time,codecs,json,pathlib,tempfile,signal
import pyte,hashlib
root=pathlib.Path(__file__).resolve().parents[4]
expected=json.loads((pathlib.Path(__file__).parent/'pty-acceptance.json').read_text())['binary_sha256']
assert hashlib.sha256((root/'bin/dotfiles').read_bytes()).hexdigest()==expected,'Rebuild the recorded binary before replaying this historical acceptance harness'
home=pathlib.Path(tempfile.mkdtemp(prefix='dotfiles-pty-'))
os.chmod(home,0o700)
(home/'.zshrc').write_text('export QA_ORIGINAL=1\n');os.chmod(home/'.zshrc',0o600)
empty=home/'empty-bin';empty.mkdir(mode=0o700)
output=root/'output/pty-release-gates';output.mkdir(parents=True,exist_ok=True)
env={'HOME':str(home),'XDG_CONFIG_HOME':str(home/'.config'),'PATH':str(empty),'TERM':'xterm-256color','COLORTERM':'truecolor','LANG':'en_US.UTF-8'}
pid,fd=pty.fork()
if pid==0:os.execve(str(root/'bin/dotfiles'),['dotfiles','--skip-intro','manage'],env)
fcntl.ioctl(fd,termios.TIOCSWINSZ,struct.pack('HHHH',24,80,0,0))
screen=pyte.Screen(80,24);stream=pyte.Stream(screen);decoder=codecs.getincrementaldecoder('utf-8')('replace')
def read(seconds=0.7):
 deadline=time.monotonic()+seconds
 while time.monotonic()<deadline:
  ready,_,_=select.select([fd],[],[],0.05)
  if ready:
   try:data=os.read(fd,65536)
   except OSError:return
   if not data:return
   with (output/'raw.log').open('ab') as f:f.write(data)
   if b'\x1b[6n' in data:os.write(fd,b'\x1b[1;1R')
   if b'\x1b]11;?' in data:os.write(fd,b'\x1b]11;rgb:0000/0000/0000\x1b\\')
   stream.feed(decoder.decode(data))
def snap(name):
 text='\n'.join(screen.display);(output/(name+'.txt')).write_text(text);print(name+': '+text[:350].replace('\n',' / '));return text
def key(value,delay=.6):
 os.write(fd,value);read(delay)
def expect(text,name):
 value=snap(name);assert text in value,(name,text,value)
try:
 read(2);expect('TOOLS','01-manage')
 key(b'2');expect('User Profiles','02-users')
 key(b'n')
 for c in 'qauser':key(c.encode(),.06)
 key(b'\r',1);expect('qauser','06-user-created')
 profiles=list((home/'.config/dotfiles').rglob('*qauser*'));assert profiles,profiles
 before={str(p):p.read_bytes() for p in profiles if p.is_file()};assert before
 key(b'n')
 for c in 'qauser':key(c.encode(),.06)
 key(b'\r',1);expect('exists','07-duplicate-refused')
 assert before=={str(p):p.read_bytes() for p in profiles if p.is_file()}
 key(b'\r',1);expect('●  qauser','08-user-switched')
 assert any(json.loads(p.read_text()).get('active_user')=='qauser' for p in (home/'.config/dotfiles').glob('*.json'))
 key(b'3');expect('Cheatsheets','03-hotkeys')
 key(b'4');expect('Package Updates','04-updates')
 key(b'5');expect('Backups','05-backups')
 key(b'n',1.5);expect('1 backup','09-backup-created')
 (home/'.zshrc').write_text('export QA_AFTER=1\n')
 key(b'\r');expect('Restore backup','10-restore-confirm')
 key(b'n');assert (home/'.zshrc').read_text()=='export QA_AFTER=1\n'
 key(b'\r');key(b'y',1.5);snap('11-restore-complete')
 assert (home/'.zshrc').read_text()=='export QA_ORIGINAL=1\n'
 key(b'd');expect('Delete backup','12-delete-confirm');key(b'n')
 assert len(list((home/'.config/dotfiles/backups').glob('*/manifest.txt')))==1
 fcntl.ioctl(fd,termios.TIOCSWINSZ,struct.pack('HHHH',32,100,0,0));screen.resize(lines=32,columns=100);read(.7);expect('Backups','13-resized')
 print('PTY assertions passed: tabs, create-only profiles, switch, backup create/cancel/restore/delete-cancel, resize')

finally:
 os.write(fd,b'\x03');read(.3)
 try:print("child status",os.waitpid(pid,0))
 finally:os.close(fd)
(output/'context.json').write_text(json.dumps({'home':str(home),'pid':pid,'source_commit':'4a4cf57'},indent=2))
