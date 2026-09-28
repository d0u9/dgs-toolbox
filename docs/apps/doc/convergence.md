# Convergence — proposal

**State: proposal, not decided.** Nothing here is built, and nothing here
overrides [`index.md`](index.md). Each section becomes design only when it is
confirmed, and is then moved into `index.md`.

The model has grown one feature at a time: a rule gained `query`, `exclude`,
`query_types`, `exclude_types`, `skip`, then `layouts` with `when`. Templates
grew their own names for the same things. Rules now need fallbacks such as
`{name|subject|type:zh}` and `{signed|due:compact|date:compact}` only because
fields diverge. The aim is fewer concepts that recur and compose.

## What stays as it is

Rules and Snapshots are two peer ways of **generating** a tree: a rule picks
Items by conditions and places them by paths, and follows them as they change;
a Snapshot fixes its contents. An Outline only **assembles** finished trees at
folders. It sees whole trees, never how one was generated. The Rules,
Snapshots and Outlines pages stay separate. Everything below is inside a rule,
or in the Templates.

## 1. Types that inherit

Types form a tree, as classes do. A field is defined once, in the type where
its meaning starts, and every type below inherits it. Relations between Items
are fields of `type: item` (`about`); paths and conditions read through them
(`about.address`). Rules then place Items like an if/else over the tree
(section 3).

The 23 Templates in use name one meaning several ways, which this removes:

| Meaning | Names now | One name |
| --- | --- | --- |
| What it is called | `name` `subject` `card_name` | `name` |
| The other party | `issuer` `provider` `payee` `party` `landlord` `school` `translator` | `issuer` |
| Its own date | `date` `issued` `signed` `paid` `registered` | `date` |
| The span it covers | `start/end` `period_start/period_end` `effective/expires` `issued/expires` | `start` `end` |
| Which sort | `category` `service` `insurance_type` `visa_type` `card_type` `class` | `category` |

`category`, not `kind`: `kind` already says whether a type is a `record` or a
`document`.

```yaml
# templates/record.yaml
type: record
abstract: true            # no Item is of this type; it only passes fields down
fields:
  - {key: owner, type: select, options: [alex, emma], required: true}
  - {key: country, type: country, format: zh, required: true}
  - {key: name}
  - {key: issuer}
  - {key: number}
  - {key: date, type: date}

# templates/money.yaml
type: money
extends: record
abstract: true
fields:
  - {key: amount}
  - {key: currency, type: select, options: [AUD, CNY, USD]}
  - key: category
    values: {utility: [水, 电, 气, 网], housing: [房租, 物业, 车位]}
  - {key: about, type: item, match: {anchor: =true}}

# templates/bill.yaml
type: bill
extends: money
fields:
  - {key: start, type: date}
  - {key: end, type: date}
  - {key: due, type: date}
```

- One parent per type. A type inherits its parent's fields, `required`,
  `distinguishing` and `defaults`.
- A type may add fields, make an inherited field required or distinguishing,
  and give it its own description. It may not rename it or change its type.
- A value may sit in a group (`utility` holds 水 and 电). A condition on the
  group matches every value in it.
- A condition on a type matches its descendants: `type is money` matches
  bill, invoice and payment.

### Draft tree

From the Templates in the tree now. Each line names the fields it adds.

```
record         owner country name issuer number date
├─ credential  end                               (abstract)
│  ├─ id_card, temp_id, student_id
│  ├─ social_card        member_number
│  ├─ bank_card          category last_four
│  ├─ driver_licence     category address card_number
│  ├─ visa               category first_entry_by travel_until
│  ├─ certificate        level
│  ├─ achievement        level
│  ├─ diploma            level major entered graduated
│  └─ marriage_cert      spouse
├─ money       amount currency category about    (abstract)
│  ├─ bill               address account start end due
│  ├─ invoice            purpose
│  └─ payment            via
├─ agreement   start end about                   (abstract)
│  ├─ tenancy            address
│  ├─ contract           category address
│  └─ insurance_policy   category insured
├─ official_letter       about
├─ notarial_certificate
├─ tax_document          category fy signed
├─ translation           of original language
└─ other                 about
```

Open:

- A credential's `date` is when it was issued, and it keeps no `start`. A
  driver licence whose card and first licence differ loses one date.
- A marriage certificate has two dates, `registered` and the copy's `issued`.
  Which one is `date`?
- `reference` (a second number on a letter, payment, visa, policy) merges
  into `number`, or stays beside it.
- `tax_document.signed` is yes/no, not a date: it keeps its name, or becomes
  a tag.
- Whether abstract types appear as filters in Browse.

## 2. Conditions as one expression

```yaml
when: type in [bill, invoice, payment] and kind in [水, 电, 气, 网] and not tags has archived
```

- `and`, `or`, `not`, parentheses.
- `is`, `in [...]`, `contains`, `has` (the field is filled, or the tag is on).
- Keys as a path has them: the Item's own, linked ones such as `about.type`,
  and those `inherit` supplies.
- One package, `internal/doc/expr`, used by rules; later by Browse's filter and
  Cases.
- It replaces `query`, `exclude`, `query_types`, `exclude_types` and `skip`.
  The old keys are removed, not rewritten on open.

## 3. A rule's paths nest

A rule is a node: a condition, a path fragment appended to its parent's, and
children. The first child whose condition an Item matches places it; one no
child takes is placed by the node itself. Settings (`inherit`, `default`,
orders) are inherited down; an order is shared by the whole rule. A child with
no path leaves what it matches out, which is what `exclude` and `skip` are
now. It generalises the `layouts` already built (6f56c06).

```yaml
name: rental
when: owner is alex and tags in [network-5, network-6, network-7]
path: 11-Rental/{#}-{about.address}/{about.start:compact}-{about.end:compact}
file: '{#}-{name}{/#}[-{date:compact}].{ext}'
children:
  - when: kind in [水, 电, 气, 网]
    path: utility/{kind}
  - when: kind in [房租, 物业, 车位]
    path: rental/{kind}
```

The Rules page edits the children where More paths is now, nested.

## Migration

No backward compatibility. The code reads only the new model: no old keys, no
rewrite on open, no rename action. The real tree (Templates, sidecars, rules)
is migrated once, by a one-off step done with the user when the new model is
built, and is not part of `dgs`.

## Order

1. Types that inherit — most fallbacks go away.
2. The expression.
3. Nested paths.
4. The one-off migration of the real tree.

Each section is written into `index.md` as it is confirmed, then built.
