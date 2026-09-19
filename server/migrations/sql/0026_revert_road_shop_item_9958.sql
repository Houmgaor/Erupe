-- Revert 0024_fix_road_shop_item_9958.sql, which was based on a
-- misidentification of the item.
--
-- Item 9958 is スペリアチケット (Superior Ticket), a Road shop reward gated
-- behind a Fatalis kill count -- not a bulk consumable. Its seeded row
-- (cost=20, quantity=1, road_fatalis=999) was correct: the nonzero
-- road_fatalis is the gate the client renders as a kill requirement, not a
-- stray value in an "always-zero" column. 0024 read it as corruption and
-- rewrote the row to cost=1/quantity=999/road_fatalis=0, which turned a
-- gated single ticket into an ungated 999x bulk purchase.
--
-- Restore the original values, but only where the row still looks exactly
-- like what 0024 wrote, so any deployment that has since retuned this item
-- by hand keeps its own values.

UPDATE public.shop_items
   SET cost = 20,
       quantity = 1,
       road_fatalis = 999
 WHERE shop_type = 10
   AND shop_id = 8
   AND item_id = 9958
   AND cost = 1
   AND quantity = 999
   AND road_fatalis = 0;
