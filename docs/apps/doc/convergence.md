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

## 1. One field catalogue

The 23 Templates in use name one meaning several ways:

| Meaning | Names now |
| --- | --- |
| The document's own date | `date` `issued` `signed` `paid` `registered` |
| The span it covers | `start/end` `period_start/period_end` `effective/expires` `issued/expires` |
| The other party | `issuer` `provider` `payee` `party` `landlord` `school` `translator` |
| What it is called | `name` `subject` `card_name` |
| Which kind | `category` `service` `insurance_type` `visa_type` `card_type` `class` `level` |
| A number | `number` `reference` `licence_number` `card_number` `account` `member_number` |

Proposal: a tree defines each field once, and a key means the same in every
Template.

```yaml
# templates/fields.yaml
owner:    {type: select, options: [alex, emma]}
country:  {type: country}
name:     {}                 # what it is called: 合同, 物业发票, IELTS
kind:     {}                 # which kind: 水, a visa subclass, a policy type
issuer:   {}                 # the other party: who issued it, was paid, signed
number:   {}
date:     {type: date}       # its own date: issued, signed, paid
start:    {type: date}
end:      {type: date}       # the end of what it covers, validity included
amount:   {}
currency: {type: select, options: [AUD, CNY, USD]}
about:    {type: item, match: {anchor: =true}}
```

- A Template lists keys from the catalogue with its own `required`,
  `distinguishing` and description: `fields: [owner, issuer, date, amount]`.
- A field one Template needs alone, such as a visa's `first_entry_by`, still
  goes into the catalogue first, so divergence is visible where it happens.
- Rules write `{name}` and `{date}`, and condition on `kind`, the same for a
  bill, an invoice and a payment.
- Old keys (`provider`, `period_start`) are not read. The real tree is
  migrated once, separately (see [Migration](#migration)).

Open: where `date`, `kind` and `issuer` end — for example whether a bill's
`due` is its `date`, and whether `expires` is always `end`.

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

1. The field catalogue — most fallbacks go away.
2. The expression.
3. Nested paths.
4. The one-off migration of the real tree.

Each section is written into `index.md` as it is confirmed, then built.
