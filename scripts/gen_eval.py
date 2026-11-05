import json, random
random.seed(7)
M = [("Swiggy","food"),("Zomato","food"),("Cafe Coffee Day","food"),("Dominos Pizza","food"),
     ("BigBasket","groceries"),("DMart","groceries"),("Reliance Fresh","groceries"),
     ("Uber","transport"),("Ola Cabs","transport"),("Indian Railways","transport"),
     ("Apollo Pharmacy","health"),("MedPlus","health"),("Max Hospital","health"),
     ("Myntra","shopping"),("Decathlon","shopping"),("Croma","shopping"),
     ("Airtel","utilities"),("Jio Fiber","utilities"),("BSES Electricity","utilities"),
     ("PVR Cinemas","entertainment"),("BookMyShow","entertainment"),("Netflix","entertainment")]
MON = ["Jan","Feb","Mar","Apr","May","Jun","Jul","Aug","Sep","Oct","Nov","Dec"]
def noisy_merchant(n):
    s = n.upper()
    if random.random() < .5:
        s = s.replace("I","1",1) if "I" in s else s.replace("O","0",1)
    return s
def noisy_amt(v):
    t = f"{v:.2f}"
    if random.random() < .3: t = t.replace("0","O",1)
    return t
cases=[]
for i,(m,c) in enumerate(random.sample(M, len(M))[:22]):
    day=random.randint(13,28); mon=random.randint(1,10); yr=2026
    fmt=random.choice(["slash","dash","text","iso"])
    ds={"slash":f"{day:02d}/{mon:02d}/{yr}","dash":f"{day:02d}-{mon:02d}-{yr}",
        "text":f"{MON[mon-1]} {day}, {yr}","iso":f"{yr}-{mon:02d}-{day:02d}"}[fmt]
    items=[(random.choice(["Item A","Service fee","Pack","Ticket","Plan","Order"]),round(random.uniform(20,900),2)) for _ in range(random.randint(1,3))]
    tax=round(sum(a for _,a in items)*.05,2); total=round(sum(a for _,a in items)+tax,2)
    lines=[noisy_merchant(m), ds]+[f"{n} {a:.2f}" for n,a in items]+[f"GST 5% {tax:.2f}", random.choice(["Total","TOTAL","Grand Total Rs","Amount payable"])+" "+noisy_amt(total)]
    cases.append({"name":f"{m.lower().replace(' ','_')}_{fmt}","ocr_text":"\n".join(lines),
      "expected":{"merchant":m,"date":f"{yr}-{mon:02d}-{day:02d}","total":total,"category":c}})
rejects=[("no_total","CORNER STORE\n2026-10-09\nBread 40\nMilk 60\n(total smudged)"),
 ("no_date","TEA POST\nChai 30\nSamosa 20\nTotal Rs 50.00"),
 ("not_a_receipt","Meeting notes: discuss roadmap with team on Friday. Action items: update docs."),
 ("garbage","@@## ~~ 1x1x ???? ||| ..")]
for n,t in rejects: cases.append({"name":"reject_"+n,"ocr_text":t,"expected":{"should_save":False}})
json.dump(cases,open("testdata/receipts_extra.json","w"),indent=1)
print(len(cases))
