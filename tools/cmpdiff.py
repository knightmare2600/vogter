# Line-diff screens captured by compare.sh, ignoring clocks and chart dots.
# Run with python3 -I, passing the work directory.
import re,sys
S=sys.argv[1]
for f in ["dash","hosts","hosts2","hostfilter","problems","problems2","sev","hide","ack","acked","help","host","host6h","host7d","items","itemfilter","back"]:
  a=open(f"{S}/cmp-zab-{f}.txt",encoding="utf-8").read().split("\n")
  b=open(f"{S}/cmp-vog-{f}.txt",encoding="utf-8").read().split("\n")
  norm=lambda l:re.sub(r"\s+"," ",l.replace("zabterm","vægter").replace(" o web ",""))
  norm2=lambda l:re.sub(r"\d+ms|\d\d:\d\d:\d\d|\d\d:\d\d|\b\d+s ago|[⠀-⣿]+","#",norm(l))
  d=[(i,x,y) for i,(x,y) in enumerate(zip(a,b)) if norm2(x)!=norm2(y)]
  print(f"== {f}: {len(d)} of {len(a)} lines differ")
  for i,x,y in d[:5]:
    print(f"  {i:2} Z {x[:125]}\n     V {y[:125]}")
