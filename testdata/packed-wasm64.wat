(module
  (import "wago_zlib.wasm64" "compress"
    (func $compress (param i64 i64 i64 i64 i32) (result i32 i64)))
  (import "wago_zlib.wasm64" "decompress"
    (func $decompress (param i64 i64 i64 i64) (result i32 i64)))
  (import "wago_zlib.wasm64" "compress_packed"
    (func $compress_packed (param i64 i64 i64 i64 i32) (result i64)))
  (import "wago_zlib.wasm64" "decompress_packed"
    (func $decompress_packed (param i64 i64 i64 i64) (result i64)))

  (memory (export "memory") i64 1)

  (func (export "success_parity") (result i32)
    (local $legacy_status i32)
    (local $legacy_written i64)
    (local $packed i64)
    (local $decode_status i32)
    (local $decode_written i64)
    (local $packed_decode i64)

    (call $compress
      (i64.const 0) (i64.const 0)
      (i64.const 64) (i64.const 64)
      (i32.const -1))
    (local.set $legacy_written)
    (local.set $legacy_status)
    (local.set $packed
      (call $compress_packed
        (i64.const 0) (i64.const 0)
        (i64.const 192) (i64.const 64)
        (i32.const -1)))

    (call $decompress
      (i64.const 64) (local.get $legacy_written)
      (i64.const 320) (i64.const 1))
    (local.set $decode_written)
    (local.set $decode_status)
    (local.set $packed_decode
      (call $decompress_packed
        (i64.const 192) (i64.shr_u (local.get $packed) (i64.const 32))
        (i64.const 384) (i64.const 1)))

    (i32.and
      (i32.and
        (i32.eq (local.get $legacy_status) (i32.wrap_i64 (local.get $packed)))
        (i64.eq (local.get $legacy_written)
          (i64.shr_u (local.get $packed) (i64.const 32))))
      (i32.and
        (i32.and
          (i32.eqz (local.get $legacy_status))
          (i64.gt_u (local.get $legacy_written) (i64.const 0)))
        (i32.and
          (i32.and
            (i32.eqz (local.get $decode_status))
            (i64.eqz (local.get $decode_written)))
          (i64.eqz (local.get $packed_decode))))))

  (func (export "failure_parity") (result i32)
    (local $legacy_status i32)
    (local $legacy_written i64)
    (local $packed i64)

    (i32.store8 (i64.const 512) (i32.const 165))
    (i32.store8 (i64.const 640) (i32.const 165))
    (call $compress
      (i64.const 0) (i64.const 0)
      (i64.const 512) (i64.const 1)
      (i32.const 99))
    (local.set $legacy_written)
    (local.set $legacy_status)
    (local.set $packed
      (call $compress_packed
        (i64.const 0) (i64.const 0)
        (i64.const 640) (i64.const 1)
        (i32.const 99)))

    (i32.and
      (i32.and
        (i32.and
          (i32.ne (local.get $legacy_status) (i32.const 0))
          (i64.eqz (local.get $legacy_written)))
        (i32.and
          (i32.eq (local.get $legacy_status) (i32.wrap_i64 (local.get $packed)))
          (i64.eqz (i64.shr_u (local.get $packed) (i64.const 32)))))
      (i32.and
        (i32.eq (i32.load8_u (i64.const 512)) (i32.const 165))
        (i32.eq (i32.load8_u (i64.const 640)) (i32.const 165)))))
)
