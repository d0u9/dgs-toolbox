# Convergence — proposal

**State: built.** The design is in [`index.md`](index.md); this keeps how it
was reached. The Rules page draws children as nested if blocks.

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
| Its number | `number` `reference` | `number` |
| Its date, or the span it covers | `date` `issued` `signed` `paid` `start/end` `period_start/period_end` `effective/expires` `issued/expires` | `date` |
| Which sort | `category` `service` `insurance_type` `visa_type` `card_type` `class` | `category` |

`category`, not `kind`: `kind` already says whether a type is a `record` or a
`document`.

### A date is one day or a span

`date` has type `date`, whose shape is one day or a pair `[start, end]`. Each
type declares which one it takes, and a type below may narrow it but not
change it. A path reads `{date.start}` and `{date.end}`; for a single day both
are that day. There are no separate `start` and `end` fields. `{date}` and
`{date:compact}` show a span as both ends: `20250101-20251231`.

| Type | `date` |
| --- | --- |
| credential (ID, visa, licence card) | `[issued, expires]` |
| bill | `[period start, period end]`; `due` is its own field |
| agreement (tenancy, contract, policy) | `[start, end]` |
| payment, invoice, letter | one day |
| marriage_cert | one day: this copy's `issued`; `registered` is its own field |
| driver_licence | `[card start, expires]`; `first_issued` is its own field |

An open end (a certificate that never lapses) leaves `end` empty.

### Templates

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
  - {key: date, type: date, shape: [day, span]}

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
  - {key: date, shape: span}
  - {key: due, type: date, shape: day}
```

- One parent per type, named by `extends`. A type inherits its parent's
  fields, `required`, `distinguishing` and `defaults`. It does not inherit
  `names`, `description` or `kind`.
- Abstract types have no `kind`. Every concrete type states its own `kind`
  (`record` or `document`); it is required.
- A type may add fields, make an inherited field required or distinguishing,
  narrow a date's shape, and give a field its own description. It may not
  rename a field or change its type.
- The tree is not fixed: changing `extends` changes only which fields a type
  inherits and what `type is X` matches. Sidecars are untouched; a type that
  loses a field its Items use fails validation.
- A value may sit in a group (`utility` holds 水 and 电). A condition on the
  group matches every value in it. All values and groups are written in the
  type that defines the field; a type below cannot add to them.
- A condition on a type matches its descendants: `type is money` matches
  bill, invoice and payment.
- A yes/no a file name shows stays a field, not a tag: a file name reads
  fields only. `tax_document.signed` is a select with one option, `已签`, and
  `[-{signed}]` adds the suffix only when it is set.
- Abstract types do not appear as filters in Browse, for now.

### Draft tree

From the Templates in the tree now. Each line names the fields it adds.

```
record         owner country name issuer number date
├─ credential                                    (abstract, date: span)
│  ├─ id_card, temp_id, student_id
│  ├─ social_card        member_number
│  ├─ bank_card          category last_four
│  ├─ driver_licence     category address card_number first_issued
│  ├─ visa               category first_entry_by travel_until
│  ├─ certificate        level
│  ├─ achievement        level
│  ├─ diploma            level major entered graduated
│  └─ marriage_cert      spouse registered       (date: day)
├─ money       amount currency category about    (abstract)
│  ├─ bill               address account due     (date: span)
│  ├─ invoice            purpose                 (date: day)
│  └─ payment            via                     (date: day)
├─ agreement   about                             (abstract, date: span)
│  ├─ tenancy            address
│  ├─ contract           category address
│  └─ insurance_policy   category insured
├─ official_letter       about
├─ notarial_certificate
├─ tax_document          category fy signed
├─ translation           of original language
└─ other                 about
```

## 2. Conditions as one expression

```yaml
if: type is money and category is utility and not tags has archived
```

- Written under `if`, which replaces `when`. `and`, `or`, `not`, parentheses.
- `is`, `in [...]`, `contains`, `has` (the field is filled, or the tag is on).
- `tags in [a, b]` holds when any of them is on. All of them is
  `tags has a and tags has b`; no operator of its own until it is needed.
- Keys as a path has them: the Item's own, linked ones such as `about.type`,
  and those `inherit` supplies.
- One package, `internal/doc/expr`, used by rules; later by Browse's filter and
  Cases.
- It replaces `query`, `exclude`, `query_types`, `exclude_types` and `skip`.
  The old keys are removed, not rewritten on open.

## 3. A rule's paths nest

A rule is a node: an `if`, a path fragment appended to its parent's, and
children. `path`, `file` and settings pass down; a child that writes its own
`file` (or setting) replaces the one above it for everything it takes. The
children are an if/elif/else chain: the first whose `if` an Item
matches takes it, and a child with no `if` is the `else`. An Item no child
takes is placed by the node itself. Settings (`inherit`, `default`, orders)
are inherited down; an order is shared by the whole rule. A child with no
path leaves what it takes out, which is what `exclude` and `skip` are now. It
generalises the `layouts` already built (6f56c06).

```yaml
name: rental
if: owner is alex and tags in [network-5, network-6, network-7]
path: 11-Rental/{#}-{about.address}/{about.date.start:compact}-{about.date.end:compact}
file: '{#}-{name}{/#}[-{date:compact}].{ext}'
children:
  - if: category is utility
    path: utility/{category}
  - if: category is housing
    path: rental/{category}
  - if: type is translation        # left out: no path
  - path: other                    # else
```

Open: how the Rules page edits nested children. Decided when section 3 is built.

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
