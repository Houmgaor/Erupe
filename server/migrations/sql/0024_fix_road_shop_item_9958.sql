-- Fix a mistranscribed Road (Hunter's Road) shop row for item 9958. It was
-- seeded as cost=20/quantity=1 with a stray value sitting in the otherwise
-- always-zero road_fatalis column, instead of the intended "999 for 1 Road
-- Point" bulk pricing (cost=1, quantity=999). This is the only row in the
-- whole table with a nonzero road_fatalis. Confirmed present with
-- road_fatalis=999 in the seed file and a 2025-10-11 DB dump, but with
-- road_fatalis=100 on the live prod DB as of 2026-07 -- the stray value has
-- drifted (manual edit?), so match on the row identity instead of the
-- specific leftover value.

UPDATE public.shop_items
   SET cost = 1,
       quantity = 999,
       road_fatalis = 0
 WHERE shop_type = 10
   AND shop_id = 8
   AND item_id = 9958
   AND road_fatalis <> 0;
