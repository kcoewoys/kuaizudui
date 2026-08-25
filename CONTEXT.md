# Activity Sharing

This context defines how published activity content participates in ordinary and priority circulation, and how each kind of usage is counted.

## Language

**Activity Publication**:
A user's single current piece of content for one activity type. Republishing updates that publication rather than creating another one.

**Ordinary Queue**:
The per-activity circulation containing each publisher with positive Ordinary Credit once. Publications take turns being offered to other users; a publication with no Ordinary Credit is excluded or skipped.
_Avoid_: Current queue, queue count

**Ordinary Credit**:
The remaining ordinary-queue quota a publisher has earned. A successful One-click Claim gives the claimant one Ordinary Credit; a delivery from the Ordinary Queue consumes one credit from the content owner. It is always `Claim Count - Ordinary Rounds`. A publisher with zero Ordinary Credit is removed or skipped. A new publication starts with zero credit. This value is internal and is not displayed as Ordinary Rounds.
_Avoid_: Cumulative ordinary deliveries, ordinary rounds

**Ordinary Round**:
The cumulative number of times a publication has been delivered from the Ordinary Queue. A successful ordinary delivery increments the content owner's Ordinary Round by one.
_Avoid_: Remaining credit, queue position

**Priority Queue**:
The per-activity circulation containing publishers with Priority Credit. It is considered before the Ordinary Queue.
_Avoid_: Accelerated queue

**Priority Credit**:
Points a publisher has committed to Priority Queue participation but that have not yet funded a priority delivery. A publisher with no Priority Credit is not a Priority Queue participant.
_Avoid_: Boosts remaining

**Priority Round**:
One successful delivery of a publication from the Priority Queue. Each Priority Round consumes one Priority Credit.
_Avoid_: Boost count

**Points Committed**:
The cumulative number of account points a publisher has confirmed for Priority Queue participation.
_Avoid_: Point cost

**Claim Count**:
The number of times a user successfully receives another user's publication on an activity page.
_Avoid_: Publication use count

**One-click Claim**:
A request for another user's publication. It selects the Priority Queue first, falls back to the Ordinary Queue, never returns the claimant's own publication, increments the claimant's Claim Count, and grants the claimant one Ordinary Credit after success. The delivered content owner's corresponding Ordinary or Priority Round is incremented.
