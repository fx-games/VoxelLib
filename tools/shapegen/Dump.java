import com.mojang.blaze3d.vertex.PoseStack;
import com.mojang.math.Transformation;
import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.util.Locale;
import net.minecraft.client.model.geom.ModelPart;
import net.minecraft.client.model.geom.builders.LayerDefinition;
import net.minecraft.client.model.geom.builders.MeshDefinition;
import net.minecraft.core.Direction;
import org.joml.Vector3f;

// Dump bakes a LayerDefinition from the game and prints every cube face as
// JSON: part path, cube index, normal and each vertex as its position in
// block pixels, its UV (0..1) and its position in the cube's own part (px,
// before the part and renderer poses: tojson recovers rotated cubes from it),
// placed by the renderer's own transform.
// Or: "dyes" prints each dye's name and texture diffuse colour (banners);
// "xf" xfClass field prints a static Transformation as a 4x4 row-major matrix.
// Args: modelClass factory[=true|false] [xfClass xfMember (dir <name> | int <n> | float <f> | field)]
// A factory returning a MeshDefinition is a 64x64 texture layer (as
// LayerDefinitions creates the piglin head's).
public class Dump {
    public static void main(String[] a) throws Exception {
        if (a[0].equals("dyes")) {
            StringBuilder o = new StringBuilder("{");
            for (net.minecraft.world.item.DyeColor d : net.minecraft.world.item.DyeColor.values())
                o.append(String.format(Locale.ROOT, "\"%s\":\"#%06X\",", d.getName(), d.getTextureDiffuseColor() & 0xFFFFFF));
            o.setLength(o.length() - 1);
            System.out.println(o.append("}"));
            return;
        }
        if (a[0].equals("xf")) {
            Field x = Class.forName(a[1]).getDeclaredField(a[2]);
            x.setAccessible(true);
            org.joml.Matrix4f m = ((Transformation) x.get(null)).getMatrixCopy();
            StringBuilder o = new StringBuilder("[");
            for (int r = 0; r < 4; r++) for (int c = 0; c < 4; c++) o.append(String.format(Locale.ROOT, "%.6f,", m.get(c, r)));
            o.setLength(o.length() - 1);
            System.out.println(o.append("]"));
            return;
        }
        String[] fa = a[1].split("=");
        Method f = fa.length > 1 ? Class.forName(a[0]).getDeclaredMethod(fa[0], boolean.class) : Class.forName(a[0]).getDeclaredMethod(fa[0]);
        f.setAccessible(true);
        Object def = fa.length > 1 ? f.invoke(null, Boolean.parseBoolean(fa[1])) : f.invoke(null);
        LayerDefinition layer = def instanceof MeshDefinition m ? LayerDefinition.create(m, 64, 64) : (LayerDefinition) def;
        ModelPart root = layer.bakeRoot();
        // $DUMP_ANIM "<Model class> unit|<float>": pose the parts as the
        // renderer does, by that Model's setupAnim (statues flip their root,
        // the shulker box sets its lid).
        String anim = System.getenv("DUMP_ANIM");
        if (anim != null && !anim.isEmpty()) {
            String[] w = anim.split(" ");
            Class<?> mc = Class.forName(w[0]);
            java.lang.reflect.Constructor<?> k = mc.getDeclaredConstructor(ModelPart.class);
            k.setAccessible(true);
            Object model = k.newInstance(root);
            Object arg = w[1].equals("unit") ? net.minecraft.util.Unit.INSTANCE : (Object) Float.valueOf(w[1]);
            Method sa = mc.getMethod("setupAnim", Object.class);
            sa.setAccessible(true);
            sa.invoke(model, arg);
        }
        PoseStack ps = new PoseStack();
        if (a.length > 2 && a[4].equals("field")) {
            Field x = Class.forName(a[2]).getDeclaredField(a[3]);
            x.setAccessible(true);
            ps.mulPose((Transformation) x.get(null));
        } else if (a.length > 2) {
            Class<?> argType = switch (a[4]) { case "dir" -> Direction.class; case "int" -> int.class; default -> float.class; };
            Object arg = switch (a[4]) { case "dir" -> Direction.valueOf(a[5].toUpperCase(Locale.ROOT)); case "int" -> Integer.parseInt(a[5]); default -> Float.parseFloat(a[5]); };
            Method x = Class.forName(a[2]).getDeclaredMethod(a[3], argType);
            x.setAccessible(true);
            ps.mulPose((Transformation) x.invoke(null, arg));
        }
        StringBuilder out = new StringBuilder("[");
        root.visit(ps, (pose, path, index, cube) -> {
            for (ModelPart.Polygon p : cube.polygons) {
                Vector3f n = pose.transformNormal(new Vector3f(p.normal()), new Vector3f());
                out.append(String.format(Locale.ROOT, "{\"part\":\"%s\",\"cube\":%d,\"normal\":[%.4f,%.4f,%.4f],\"verts\":[", path, index, n.x, n.y, n.z));
                for (ModelPart.Vertex v : p.vertices()) {
                    Vector3f w = pose.pose().transformPosition(new Vector3f(v.x() / 16f, v.y() / 16f, v.z() / 16f));
                    out.append(String.format(Locale.ROOT, "[%.4f,%.4f,%.4f,%.6f,%.6f,%.4f,%.4f,%.4f],", w.x * 16, w.y * 16, w.z * 16, v.u(), v.v(), v.x(), v.y(), v.z()));
                }
                out.setLength(out.length() - 1);
                out.append("]},");
            }
        });
        if (out.length() > 1) out.setLength(out.length() - 1);
        System.out.println(out.append("]"));
    }
}
