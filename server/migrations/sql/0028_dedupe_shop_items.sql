-- Remove exact duplicate shop rows left behind by the old SQL seed.
--
-- The road shop's limited tab (shop_type 10, shop_id 7) was seeded with five
-- items twice (10750, 13508, 14705, 15027, 15028), so they show twice in game.
-- The JSON seed no longer lists them twice, but seeds only run on a fresh
-- database: servers upgraded from 9.5.0 or earlier still have the copies.
--
-- Only rows identical in every column but id are touched, in any shop, so a
-- row an operator deliberately made different is left alone. For each group
-- the lowest id is kept; purchase counters recorded against a dropped copy
-- are added onto the kept row before the copy is deleted.

CREATE TEMP TABLE shop_items_dupes ON COMMIT DROP AS
SELECT s.id AS dupe_id,
       k.keep_id
  FROM public.shop_items s
  JOIN (SELECT MIN(id) AS keep_id,
               shop_type, shop_id, item_id, cost, quantity, min_hr, min_sr,
               min_gr, store_level, max_quantity, road_floors, road_fatalis
          FROM public.shop_items
         GROUP BY shop_type, shop_id, item_id, cost, quantity, min_hr, min_sr,
                  min_gr, store_level, max_quantity, road_floors, road_fatalis
        HAVING COUNT(*) > 1) k
    ON s.shop_type    IS NOT DISTINCT FROM k.shop_type
   AND s.shop_id      IS NOT DISTINCT FROM k.shop_id
   AND s.item_id      IS NOT DISTINCT FROM k.item_id
   AND s.cost         IS NOT DISTINCT FROM k.cost
   AND s.quantity     IS NOT DISTINCT FROM k.quantity
   AND s.min_hr       IS NOT DISTINCT FROM k.min_hr
   AND s.min_sr       IS NOT DISTINCT FROM k.min_sr
   AND s.min_gr       IS NOT DISTINCT FROM k.min_gr
   AND s.store_level  IS NOT DISTINCT FROM k.store_level
   AND s.max_quantity IS NOT DISTINCT FROM k.max_quantity
   AND s.road_floors  IS NOT DISTINCT FROM k.road_floors
   AND s.road_fatalis IS NOT DISTINCT FROM k.road_fatalis
 WHERE s.id <> k.keep_id;

INSERT INTO public.shop_items_bought (character_id, shop_item_id, bought)
SELECT b.character_id, d.keep_id, SUM(b.bought)
  FROM public.shop_items_bought b
  JOIN shop_items_dupes d ON d.dupe_id = b.shop_item_id
 GROUP BY b.character_id, d.keep_id
ON CONFLICT (character_id, shop_item_id)
DO UPDATE SET bought = public.shop_items_bought.bought + EXCLUDED.bought;

DELETE FROM public.shop_items_bought
 WHERE shop_item_id IN (SELECT dupe_id FROM shop_items_dupes);

DELETE FROM public.shop_items
 WHERE id IN (SELECT dupe_id FROM shop_items_dupes);
